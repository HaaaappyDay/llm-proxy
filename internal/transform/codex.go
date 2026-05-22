package transform

// ApplyCodexUpstreamRequest patches OpenAI Responses payload for Codex backend.
func ApplyCodexUpstreamRequest(payload map[string]any, stream bool) map[string]any {
	out := make(map[string]any, len(payload)+4)
	for k, v := range payload {
		out[k] = v
	}
	out["store"] = false
	out["include"] = []string{"reasoning.encrypted_content"}
	delete(out, "max_output_tokens")
	delete(out, "temperature")
	delete(out, "top_p")
	if stream {
		out["stream"] = true
	} else {
		out["stream"] = true // Codex upstream requires streaming
	}
	return out
}

// AnthropicToCodexResponses converts Anthropic request to Codex upstream format.
func AnthropicToCodexResponses(anthropic map[string]any) map[string]any {
	unified := RequestFromAnthropic(anthropic)
	responses := RequestToOpenAIResponses(unified)
	return ApplyCodexUpstreamRequest(responses, true)
}
