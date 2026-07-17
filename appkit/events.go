package appkit

// UnwrapNext extracts the inner streaming frame from a protocol-1 next envelope.
func UnwrapNext(event map[string]interface{}) map[string]interface{} {
	if event == nil {
		return nil
	}
	if asString(event["type"]) != "next" {
		return event
	}
	payload, _ := event["payload"].(map[string]interface{})
	if payload == nil {
		return event
	}
	if data, ok := payload["data"].(map[string]interface{}); ok && data != nil {
		return data
	}
	return event
}

func asString(v interface{}) string {
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}

func asNamespace(v interface{}) []interface{} {
	switch t := v.(type) {
	case []interface{}:
		return t
	case []string:
		out := make([]interface{}, len(t))
		for i, s := range t {
			out[i] = s
		}
		return out
	default:
		return nil
	}
}
