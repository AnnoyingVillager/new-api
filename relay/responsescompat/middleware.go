package responsescompat

import (
	"bytes"
	"io"
	"mime"
	"net/http"
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/gin-gonic/gin"
)

// Middleware applies the request repair before routing and wraps the response
// so every ordinary and task-plugin handler gets the same client-visible model.
func Middleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		if !Enabled() {
			c.Next()
			return
		}

		requestedModel := prepareRequest(c)
		originalWriter := c.Writer
		writer := newResponseWriter(originalWriter, requestedModel)
		c.Writer = writer
		defer func() {
			writer.finish()
			c.Writer = originalWriter
		}()
		c.Next()
	}
}

func prepareRequest(c *gin.Context) string {
	storage, err := common.GetBodyStorage(c)
	if err != nil {
		return ""
	}
	payload, err := storage.Bytes()
	if err != nil {
		return ""
	}
	requestedModel := RequestedModel(payload)
	if requestedModel != "" {
		// Rewriting needs a plain response body and an unbuffered SSE stream.
		c.Request.Header.Del("Accept-Encoding")
	}
	patched, err := PatchRequestPayload(payload)
	if err != nil || bytes.Equal(patched, payload) {
		return requestedModel
	}
	newStorage, err := common.CreateBodyStorage(patched)
	if err != nil {
		return requestedModel
	}
	_ = storage.Close()
	c.Set(common.KeyBodyStorage, newStorage)
	c.Set(common.KeyRequestBody, nil)
	c.Request.Body = io.NopCloser(newStorage)
	c.Request.ContentLength = int64(len(patched))
	return requestedModel
}

type responseMode uint8

const (
	responseModeRaw responseMode = iota
	responseModeJSON
	responseModeSSE
)

type compatibilityWriter struct {
	gin.ResponseWriter
	requestedModel string
	statusCode     int
	mode           responseMode
	modeResolved   bool
	headerSent     bool
	size           int
	jsonBody       bytes.Buffer
	sse            *sseTransformer
}

func newResponseWriter(writer gin.ResponseWriter, requestedModel string) *compatibilityWriter {
	return &compatibilityWriter{ResponseWriter: writer, requestedModel: requestedModel}
}

func (w *compatibilityWriter) WriteHeader(statusCode int) {
	if w.statusCode != 0 {
		return
	}
	w.statusCode = statusCode
	w.resolveMode()
	if w.mode != responseModeJSON {
		w.sendHeader()
	}
}

func (w *compatibilityWriter) WriteHeaderNow() {
	if w.statusCode == 0 {
		w.statusCode = http.StatusOK
	}
	w.resolveMode()
	if !w.modeResolved || w.mode != responseModeJSON {
		w.sendHeader()
	}
}

func (w *compatibilityWriter) Write(data []byte) (int, error) {
	if w.statusCode == 0 {
		w.statusCode = http.StatusOK
	}
	w.resolveMode()
	w.size += len(data)
	switch w.mode {
	case responseModeJSON:
		_, _ = w.jsonBody.Write(data)
		return len(data), nil
	case responseModeSSE:
		w.sendHeader()
		rewritten := w.sse.feed(data)
		if len(rewritten) == 0 {
			return len(data), nil
		}
		if _, err := w.ResponseWriter.Write(rewritten); err != nil {
			return 0, err
		}
		return len(data), nil
	default:
		w.sendHeader()
		return w.ResponseWriter.Write(data)
	}
}

func (w *compatibilityWriter) WriteString(data string) (int, error) {
	return w.Write([]byte(data))
}

func (w *compatibilityWriter) Status() int {
	if w.statusCode != 0 {
		return w.statusCode
	}
	return w.ResponseWriter.Status()
}

func (w *compatibilityWriter) Size() int {
	return w.size
}

func (w *compatibilityWriter) Written() bool {
	return w.headerSent || w.size != 0 || w.ResponseWriter.Written()
}

func (w *compatibilityWriter) Flush() {
	if w.statusCode == 0 {
		w.statusCode = http.StatusOK
	}
	w.resolveMode()
	if w.mode == responseModeJSON {
		return
	}
	w.sendHeader()
	w.ResponseWriter.Flush()
}

func (w *compatibilityWriter) Unwrap() http.ResponseWriter {
	return w.ResponseWriter
}

func (w *compatibilityWriter) finish() {
	if w.statusCode == 0 && w.size == 0 {
		return
	}
	if w.statusCode == 0 {
		w.statusCode = http.StatusOK
	}
	w.resolveMode()
	switch w.mode {
	case responseModeJSON:
		body := RewriteResponsePayload(w.jsonBody.Bytes(), w.requestedModel)
		w.Header().Del("Transfer-Encoding")
		w.Header().Set("Content-Length", strconv.Itoa(len(body)))
		w.sendHeader()
		if len(body) != 0 {
			_, _ = w.ResponseWriter.Write(body)
		}
	case responseModeSSE:
		w.sendHeader()
		if remaining := w.sse.finish(); len(remaining) != 0 {
			_, _ = w.ResponseWriter.Write(remaining)
		}
		w.ResponseWriter.Flush()
	default:
		w.sendHeader()
	}
}

func (w *compatibilityWriter) resolveMode() {
	if w.modeResolved {
		return
	}
	status := w.statusCode
	if status == 0 {
		status = http.StatusOK
	}
	encoding := strings.TrimSpace(w.Header().Get("Content-Encoding"))
	if w.requestedModel == "" || status < 200 || status >= 300 || (encoding != "" && !strings.EqualFold(encoding, "identity")) {
		return
	}
	mediaType, _, err := mime.ParseMediaType(w.Header().Get("Content-Type"))
	if err != nil {
		return
	}
	w.modeResolved = true
	switch {
	case mediaType == "text/event-stream":
		w.mode = responseModeSSE
		w.sse = &sseTransformer{model: w.requestedModel}
	case mediaType == "application/json" || strings.HasSuffix(mediaType, "+json"):
		w.mode = responseModeJSON
	}
}

func (w *compatibilityWriter) sendHeader() {
	if w.headerSent {
		return
	}
	if w.statusCode == 0 {
		w.statusCode = http.StatusOK
	}
	w.ResponseWriter.WriteHeader(w.statusCode)
	w.headerSent = true
}
