package appkit

// ShouldDropStreamChunkEarly reports whether a chunk can be skipped before the turn pipeline.
func ShouldDropStreamChunkEarly(namespace []interface{}, mode string, data interface{}) bool {
	_ = namespace
	if mode == "updates" {
		return updatesChunkIsNoop(data)
	}
	if mode == "messages" {
		return messageChunkIsNonActionable(data)
	}
	return false
}

func updatesChunkIsNoop(data interface{}) bool {
	m, ok := data.(map[string]interface{})
	if !ok || m == nil {
		return true
	}
	_, has := m["__interrupt__"]
	return !has
}

func messageChunkIsNonActionable(data interface{}) bool {
	arr, ok := data.([]interface{})
	if !ok || len(arr) != 2 {
		return false
	}
	if arr[0] == nil {
		return true
	}
	msg, ok := arr[0].(map[string]interface{})
	if !ok || msg == nil {
		return false
	}
	body := wireBody(msg)
	raw := asString(body["type"])
	if raw == "" {
		raw = asString(msg["type"])
	}
	if raw == "tool" || raw == "ToolMessage" || hasSuffix(raw, "ToolMessage") {
		return false
	}
	if dictHasToolInvocation(msg) {
		return false
	}
	if body["phase"] != nil || msg["phase"] != nil {
		return false
	}
	return plainText(msg) == ""
}

func wireBody(msg map[string]interface{}) map[string]interface{} {
	for _, key := range []string{"kwargs", "data"} {
		if nested, ok := msg[key].(map[string]interface{}); ok && nested != nil {
			return nested
		}
	}
	return msg
}

func dictHasToolInvocation(msg map[string]interface{}) bool {
	body := wireBody(msg)
	if body["tool_calls"] != nil || body["tool_call_chunks"] != nil {
		return true
	}
	for _, key := range []string{"content", "content_blocks"} {
		raw, ok := body[key].([]interface{})
		if !ok {
			continue
		}
		for _, item := range raw {
			m, ok := item.(map[string]interface{})
			if !ok {
				continue
			}
			t := asString(m["type"])
			if t == "tool_call" || t == "tool_call_chunk" || t == "tool_use" {
				return true
			}
		}
	}
	return false
}

func plainText(msg map[string]interface{}) string {
	body := wireBody(msg)
	content := body["content"]
	if content == nil {
		content = msg["content"]
	}
	switch t := content.(type) {
	case string:
		return trimSpace(t)
	case []interface{}:
		var parts string
		for _, block := range t {
			if s, ok := block.(string); ok {
				parts += s
				continue
			}
			if m, ok := block.(map[string]interface{}); ok {
				if text, ok := m["text"].(string); ok {
					parts += text
				}
			}
		}
		return trimSpace(parts)
	default:
		return ""
	}
}

func hasSuffix(s, suffix string) bool {
	return len(s) >= len(suffix) && s[len(s)-len(suffix):] == suffix
}

func trimSpace(s string) string {
	start, end := 0, len(s)
	for start < end && (s[start] == ' ' || s[start] == '\t' || s[start] == '\n' || s[start] == '\r') {
		start++
	}
	for end > start && (s[end-1] == ' ' || s[end-1] == '\t' || s[end-1] == '\n' || s[end-1] == '\r') {
		end--
	}
	return s[start:end]
}
