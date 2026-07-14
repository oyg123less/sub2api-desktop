package gateway

import "testing"

func TestAPIKeyResponsesURL(t *testing.T) {
	cases := map[string]string{
		"https://ai.example.com":                          "https://ai.example.com/v1/responses",
		"https://ai.example.com/":                         "https://ai.example.com/v1/responses",
		"https://ai.example.com/v1":                       "https://ai.example.com/v1/responses",
		"https://ai.example.com/v1/":                      "https://ai.example.com/v1/responses",
		"https://ai.example.com/v1/responses":             "https://ai.example.com/v1/responses",
		"https://chatgpt.com/backend-api/codex/responses": "https://chatgpt.com/backend-api/codex/responses",
		"https://ai.example.com/openai/v2":                "https://ai.example.com/openai/v2/responses",
	}
	for in, want := range cases {
		if got := apiKeyResponsesURL(in); got != want {
			t.Errorf("apiKeyResponsesURL(%q) = %q, want %q", in, got, want)
		}
	}
}
