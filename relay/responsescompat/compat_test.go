package responsescompat

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/MoeForge/moego/ojson"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestPatchRequestPayloadMovesIDToCallID(t *testing.T) {
	t.Setenv(EnabledEnv, "true")
	t.Setenv(CallIDModeEnv, "move")
	payload := []byte(`{"model":"alias","input":[{"type":"function_call_output","id":"call_123","output":"ok"},{"type":"custom_tool_call_output","call_id":"existing","id":"ignored","output":"ok"},{"type":"custom_tool_call_output","id":"call_custom","output":"ok"}]}`)

	patched, err := PatchRequestPayload(payload)
	require.NoError(t, err)
	input := ojson.GetBytesNoCopy(patched, "input").Array()
	require.Equal(t, "call_123", input[0].Get("call_id").String())
	require.False(t, input[0].Get("id").Exists())
	require.Equal(t, "existing", input[1].Get("call_id").String())
	require.Equal(t, "ignored", input[1].Get("id").String())
	require.Equal(t, "call_custom", input[2].Get("call_id").String())
	require.False(t, input[2].Get("id").Exists())
}

func TestRewriteResponsePayloadUsesProtocolFieldsOnly(t *testing.T) {
	t.Setenv(EnabledEnv, "true")
	payload := []byte(`{"model":"upstream","response":{"model":"upstream"},"metadata":{"model":"business-value"}}`)
	rewritten := RewriteResponsePayload(payload, "alias")

	require.Equal(t, "alias", ojson.GetBytesNoCopy(rewritten, "model").String())
	require.Equal(t, "alias", ojson.GetBytesNoCopy(rewritten, "response.model").String())
	require.Equal(t, "business-value", ojson.GetBytesNoCopy(rewritten, "metadata.model").String())
}

func TestRewriteResponsePayloadHonorsGlobalDisable(t *testing.T) {
	t.Setenv(EnabledEnv, "false")
	payload := []byte(`{"model":"upstream"}`)

	require.Equal(t, payload, RewriteResponsePayload(payload, "alias"))
}

func TestSSETransformerHandlesSplitEvents(t *testing.T) {
	transformer := &sseTransformer{model: "alias"}
	first := transformer.feed([]byte("event: response.created\ndata: {\"response\":{\"model\":\"upstream\"}}\n"))
	second := transformer.feed([]byte("\n: PING\n\ndata: [DONE]\n\n"))
	output := string(first) + string(second) + string(transformer.finish())

	require.Contains(t, output, `"model":"alias"`)
	require.NotContains(t, output, `"model":"upstream"`)
	require.Contains(t, output, ": PING\n\n")
	require.Contains(t, output, "data: [DONE]\n\n")
}

func TestMiddlewareRewritesJSONAndRecalculatesLength(t *testing.T) {
	t.Setenv(EnabledEnv, "true")
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	engine.Use(Middleware())
	engine.POST("/v1/responses", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"model":    "upstream",
			"response": gin.H{"model": "upstream"},
			"metadata": gin.H{"model": "business-value"},
		})
	})

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(`{"model":"alias","input":[]}`))
	request.Header.Set("Content-Type", "application/json")
	engine.ServeHTTP(recorder, request)

	require.Equal(t, http.StatusOK, recorder.Code)
	require.Equal(t, "alias", ojson.GetBytesNoCopy(recorder.Body.Bytes(), "model").String())
	require.Equal(t, "alias", ojson.GetBytesNoCopy(recorder.Body.Bytes(), "response.model").String())
	require.Equal(t, "business-value", ojson.GetBytesNoCopy(recorder.Body.Bytes(), "metadata.model").String())
	require.Equal(t, int64(len(recorder.Body.Bytes())), recorder.Result().ContentLength)
}

func TestMiddlewareRewritesFlushedSSEChunks(t *testing.T) {
	t.Setenv(EnabledEnv, "true")
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	engine.Use(Middleware())
	engine.POST("/v1/responses", func(c *gin.Context) {
		c.Header("Content-Type", "text/event-stream")
		_, _ = c.Writer.WriteString("event: response.created\ndata: {\"response\":{\"model\":\"up")
		c.Writer.Flush()
		_, _ = c.Writer.WriteString("stream\"}}\n\ndata: [DONE]\n\n")
		c.Writer.Flush()
	})

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(`{"model":"alias","stream":true}`))
	request.Header.Set("Content-Type", "application/json")
	engine.ServeHTTP(recorder, request)

	require.True(t, recorder.Flushed)
	require.Equal(t, http.StatusOK, recorder.Code)
	require.Contains(t, recorder.Body.String(), `"model":"alias"`)
	require.NotContains(t, recorder.Body.String(), `"model":"upstream"`)
	require.Contains(t, recorder.Body.String(), "data: [DONE]\n\n")
}
