package config

import "testing"

func TestEnvBool(t *testing.T) {
	t.Setenv("LLM_PROXY_DEBUG", "1")
	if !envBool("LLM_PROXY_DEBUG") {
		t.Fatal("envBool should accept 1")
	}

	t.Setenv("LLM_PROXY_DEBUG", "true")
	if !envBool("LLM_PROXY_DEBUG") {
		t.Fatal("envBool should accept true")
	}

	t.Setenv("LLM_PROXY_DEBUG", "0")
	if envBool("LLM_PROXY_DEBUG") {
		t.Fatal("envBool should reject 0")
	}
}
