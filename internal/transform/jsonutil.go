package transform

import (
	"encoding/json"
	"strconv"
	"strings"
)

// asMapSlice normalizes JSON-decoded or in-memory slice fields to []map[string]any.
func asMapSlice(v any) []map[string]any {
	switch t := v.(type) {
	case []map[string]any:
		return t
	case []any:
		out := make([]map[string]any, 0, len(t))
		for _, item := range t {
			if m, ok := item.(map[string]any); ok {
				out = append(out, m)
			}
		}
		return out
	default:
		return nil
	}
}

func numberFloat(v any) (float64, bool) {
	switch n := v.(type) {
	case float64:
		return n, true
	case json.Number:
		f, err := n.Float64()
		return f, err == nil
	case int:
		return float64(n), true
	case int64:
		return float64(n), true
	case string:
		f, err := strconv.ParseFloat(n, 64)
		return f, err == nil
	default:
		return 0, false
	}
}

func numberInt(v any) (int, bool) {
	f, ok := numberFloat(v)
	if !ok {
		return 0, false
	}
	return int(f), true
}

func asAnySlice(v any) []any {
	switch t := v.(type) {
	case []any:
		return t
	case []map[string]any:
		out := make([]any, len(t))
		for i, m := range t {
			out[i] = m
		}
		return out
	default:
		return nil
	}
}

func splitDataURL(url string) (string, string, bool) {
	const marker = ";base64,"
	if !strings.HasPrefix(url, "data:") {
		return "", "", false
	}
	rest := strings.TrimPrefix(url, "data:")
	i := strings.Index(rest, marker)
	if i < 0 {
		return "", "", false
	}
	return rest[:i], rest[i+len(marker):], true
}
