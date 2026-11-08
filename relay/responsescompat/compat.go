package responsescompat

import (
	"bytes"
	"fmt"
	"os"
	"slices"
	"strings"

	"github.com/MoeForge/moego/ojson"
)

const (
	// EnabledEnv disables the fork-specific Responses compatibility behavior.
	EnabledEnv = "RESPONSES_COMPAT_ENABLED"
	// CallIDModeEnv selects how an id-only function_call_output is repaired.
	CallIDModeEnv = "RESPONSES_CALL_ID_MODE"
)

const (
	callIDModeFill = "fill"
	callIDModeMove = "move"
	callIDModeOff  = "off"
)

var toolOutputTypes = map[string]struct{}{
	"function_call_output":    {},
	"custom_tool_call_output": {},
}

// Enabled reports whether the fork-specific compatibility layer is active.
// It is enabled by default so rebuilding the image is enough to deploy it.
func Enabled() bool {
	value := strings.ToLower(strings.TrimSpace(os.Getenv(EnabledEnv)))
	return value != "0" && value != "false" && value != "off" && value != "no"
}

func callIDMode() string {
	value := strings.ToLower(strings.TrimSpace(os.Getenv(CallIDModeEnv)))
	switch value {
	case callIDModeFill, callIDModeMove, callIDModeOff:
		return value
	default:
		return callIDModeMove
	}
}

// PatchRequestPayload copies an id-only tool output correlation into call_id.
// Existing call_id values always win, and only the top-level Responses input
// array is inspected.
func PatchRequestPayload(payload []byte) ([]byte, error) {
	mode := callIDMode()
	if !Enabled() || mode == callIDModeOff || !ojson.ValidBytes(payload) {
		return payload, nil
	}
	root := ojson.GetBytesNoCopy(payload, "@this")
	input := ojson.GetBytesNoCopy(payload, "input")
	if !root.IsObject() || !input.IsArray() {
		return payload, nil
	}

	doc := ojson.NewDoc(payload)
	changed := false
	for index, item := range input.Array() {
		if !item.IsObject() {
			continue
		}
		itemType := item.Get("type")
		if itemType.Type != ojson.String {
			continue
		}
		if _, ok := toolOutputTypes[itemType.String()]; !ok {
			continue
		}
		callID := item.Get("call_id")
		if callID.Type == ojson.String && strings.TrimSpace(callID.String()) != "" {
			continue
		}
		itemID := item.Get("id")
		if itemID.Type != ojson.String || strings.TrimSpace(itemID.String()) == "" {
			continue
		}

		if err := doc.Set(fmt.Sprintf("input.%d.call_id", index), itemID.String()); err != nil {
			return payload, fmt.Errorf("set function call output call_id: %w", err)
		}
		changed = true
		if mode == callIDModeMove {
			if err := doc.Delete(fmt.Sprintf("input.%d.id", index)); err != nil {
				return payload, fmt.Errorf("remove function call output id: %w", err)
			}
		}
	}
	if !changed {
		return payload, nil
	}
	return doc.Bytes(), nil
}

// RequestedModel returns the client-visible model from a Responses request.
func RequestedModel(payload []byte) string {
	model := ojson.GetBytesNoCopy(payload, "model")
	if model.Type != ojson.String {
		return ""
	}
	return strings.TrimSpace(model.String())
}

// RewriteResponsePayload restores only the protocol-defined model fields.
// Other JSON members named model, including tool and business data, stay intact.
func RewriteResponsePayload(payload []byte, requestedModel string) []byte {
	if !Enabled() || requestedModel == "" || !ojson.ValidBytes(payload) {
		return payload
	}
	rewritten := payload
	for _, path := range []string{"model", "response.model"} {
		value := ojson.GetBytesNoCopy(rewritten, path)
		if value.Type != ojson.String || value.String() == requestedModel {
			continue
		}
		next, err := ojson.SetBytes(rewritten, path, requestedModel)
		if err != nil {
			return payload
		}
		rewritten = next
	}
	return rewritten
}

type sseTransformer struct {
	model   string
	pending bytes.Buffer
	event   bytes.Buffer
}

func (s *sseTransformer) feed(data []byte) []byte {
	s.pending.Write(data)
	var output bytes.Buffer
	for {
		buf := s.pending.Bytes()
		lineEnd := bytes.IndexByte(buf, '\n')
		if lineEnd < 0 {
			break
		}
		line := append([]byte(nil), buf[:lineEnd+1]...)
		s.pending.Next(lineEnd + 1)
		s.event.Write(line)
		if len(bytes.TrimSpace(line)) == 0 {
			output.Write(s.rewriteEvent())
		}
	}
	return output.Bytes()
}

func (s *sseTransformer) finish() []byte {
	if s.pending.Len() > 0 {
		s.event.Write(s.pending.Bytes())
		s.pending.Reset()
	}
	return s.rewriteEvent()
}

func (s *sseTransformer) rewriteEvent() []byte {
	event := append([]byte(nil), s.event.Bytes()...)
	s.event.Reset()
	lines := bytes.SplitAfter(event, []byte("\n"))
	dataLines := make([]int, 0, 1)
	values := make([][]byte, 0, 1)
	for index, line := range lines {
		content, _ := splitSSELine(line)
		if !bytes.HasPrefix(content, []byte("data:")) {
			continue
		}
		valueStart := len("data:")
		if valueStart < len(content) && content[valueStart] == ' ' {
			valueStart++
		}
		dataLines = append(dataLines, index)
		values = append(values, append([]byte(nil), content[valueStart:]...))
	}
	if len(dataLines) == 0 {
		return event
	}
	data := bytes.Join(values, []byte("\n"))
	if bytes.Equal(bytes.TrimSpace(data), []byte("[DONE]")) {
		return event
	}
	rewritten := RewriteResponsePayload(data, s.model)
	if bytes.Equal(rewritten, data) {
		return event
	}

	var output bytes.Buffer
	for index, line := range lines {
		if len(dataLines) > 1 && index != dataLines[0] && slices.Contains(dataLines, index) {
			continue
		}
		if !slices.Contains(dataLines, index) {
			output.Write(line)
			continue
		}
		content, ending := splitSSELine(line)
		prefixEnd := len("data:")
		if prefixEnd < len(content) && content[prefixEnd] == ' ' {
			prefixEnd++
		}
		output.Write(content[:prefixEnd])
		output.Write(rewritten)
		output.Write(ending)
	}
	return output.Bytes()
}

func splitSSELine(line []byte) ([]byte, []byte) {
	content := line
	if len(content) > 0 && content[len(content)-1] == '\n' {
		content = content[:len(content)-1]
	}
	if len(content) > 0 && content[len(content)-1] == '\r' {
		content = content[:len(content)-1]
	}
	return content, line[len(content):]
}
