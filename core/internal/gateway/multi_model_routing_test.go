package gateway_test

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"sub2api-desktop/core/internal/account"
	"sub2api-desktop/core/internal/gateway"
	"sub2api-desktop/core/internal/store"
)

func TestDynamicAccountPoolDiscoveryAndToggling(t *testing.T) {
	st := newTestStore(t)
	mgr := account.NewManager(st)
	e := gateway.New(st, mgr, func() store.Settings { s, _ := st.LoadSettings(); return s }, nil)

	// Initial empty pool: ActiveModels() should be empty
	if len(e.ActiveModels()) != 0 {
		t.Fatalf("ActiveModels() on empty pool = %v, want empty", e.ActiveModels())
	}

	// Add an active DeepSeek APIKey account
	dsAccount, err := st.CreateAccount(&store.Account{
		AccountType: store.AccountTypeAPIKey,
		BaseURL:     "https://api.deepseek.com",
		APIKey:      "sk-deepseek-test",
		Status:      store.AccountActive,
	})
	if err != nil {
		t.Fatal(err)
	}

	// ActiveModels should now report DeepSeek models
	active := e.ActiveModels()
	if len(active) != 2 || active[0] != "deepseek-chat" || active[1] != "deepseek-reasoner" {
		t.Fatalf("ActiveModels() after adding DeepSeek = %v", active)
	}

	// Add an active ChatGPT OAuth account
	gptAccount, err := st.CreateAccount(&store.Account{
		AccountType: store.AccountTypeOAuth,
		AccessToken: "oauth-token",
		Status:      store.AccountActive,
	})
	if err != nil {
		t.Fatal(err)
	}

	// ActiveModels should now include both GPT models and DeepSeek models
	active = e.ActiveModels()
	hasDS := false
	hasGPT6 := false
	for _, m := range active {
		if m == "deepseek-chat" {
			hasDS = true
		}
		if m == "gpt-6" {
			hasGPT6 = true
		}
	}
	if !hasDS || !hasGPT6 {
		t.Fatalf("ActiveModels() should contain both deepseek and gpt-6, got: %v", active)
	}

	// Toggle DeepSeek account to Disabled
	if err := st.SetAccountStatus(dsAccount.ID, store.AccountDisabled, "user_disabled"); err != nil {
		t.Fatal(err)
	}

	// Now DeepSeek models MUST vanish from ActiveModels
	active = e.ActiveModels()
	for _, m := range active {
		if m == "deepseek-chat" || m == "deepseek-reasoner" {
			t.Fatalf("DeepSeek model %q should not be present when account is disabled! got: %v", m, active)
		}
	}

	// Toggle GPT account to Disabled as well
	if err := st.SetAccountStatus(gptAccount.ID, store.AccountDisabled, "user_disabled"); err != nil {
		t.Fatal(err)
	}

	// Now all active models are gone
	if len(e.ActiveModels()) != 0 {
		t.Fatalf("ActiveModels() should be empty when all accounts are disabled, got: %v", e.ActiveModels())
	}
}

func TestDeepSeekResponsesBridgeForwarding(t *testing.T) {
	var capturedAuth string
	var capturedBody []byte

	// Mock upstream DeepSeek server responding with ChatCompletions SSE
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedAuth = r.Header.Get("Authorization")
		capturedBody, _ = io.ReadAll(r.Body)

		w.Header().Set("Content-Type", "text/event-stream")
		// Emit Chat Completions stream chunks with reasoning content and content
		_, _ = io.WriteString(w, `data: {"id":"chatcmpl-1","choices":[{"delta":{"role":"assistant","reasoning_content":"thinking about it..."}}]}`+"\n\n")
		_, _ = io.WriteString(w, `data: {"id":"chatcmpl-1","choices":[{"delta":{"content":"Hello from DeepSeek!"}}]}`+"\n\n")
		_, _ = io.WriteString(w, `data: {"id":"chatcmpl-1","choices":[{"finish_reason":"stop"}]}`+"\n\n")
		_, _ = io.WriteString(w, "data: [DONE]\n\n")
	}))
	defer upstream.Close()

	st := newTestStore(t)
	_, err := st.CreateAccount(&store.Account{
		AccountType: store.AccountTypeAPIKey,
		BaseURL:     upstream.URL,
		APIKey:      "sk-deepseek-secret",
		Status:      store.AccountActive,
	})
	if err != nil {
		t.Fatal(err)
	}

	e := gateway.New(st, account.NewManager(st), func() store.Settings { s, _ := st.LoadSettings(); return s }, nil)

	// Client (Codex Desktop) sends POST /v1/responses targeting deepseek-chat
	responsesPayload := map[string]any{
		"model":  "deepseek-chat",
		"stream": true,
		"input": []map[string]any{
			{"role": "user", "content": "hi"},
		},
	}
	payloadBytes, _ := json.Marshal(responsesPayload)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewReader(payloadBytes))

	e.Responses(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("Responses HTTP status = %d, body = %s", rec.Code, rec.Body.String())
	}

	if capturedAuth != "Bearer sk-deepseek-secret" {
		t.Fatalf("Upstream authorization = %q, want Bearer sk-deepseek-secret", capturedAuth)
	}

	// Verify upstream received a Chat Completions request
	var chatReq map[string]any
	if err := json.Unmarshal(capturedBody, &chatReq); err != nil {
		t.Fatalf("Failed to parse upstream chat request body: %v", err)
	}
	if chatReq["model"] != "deepseek-chat" {
		t.Fatalf("Upstream chat model = %v, want deepseek-chat", chatReq["model"])
	}

	// Verify the response was converted into Responses SSE events
	responseOutput := rec.Body.String()
	if !bytes.Contains([]byte(responseOutput), []byte("response.output_text.delta")) {
		t.Fatalf("Client output should contain response.output_text.delta events, got: %s", responseOutput)
	}
	if !bytes.Contains([]byte(responseOutput), []byte("Hello from DeepSeek!")) {
		t.Fatalf("Client output missing message text: %s", responseOutput)
	}
}

func TestAccountAvailableModelsAndProbing(t *testing.T) {
	// Mock upstream /models server
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/models" || r.URL.Path == "/models" {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"object": "list",
				"data": []map[string]any{
					{"id": "deepseek-chat"},
					{"id": "deepseek-reasoner"},
					{"id": "text-embedding-v3"},
				},
			})
			return
		}
		if r.URL.Path == "/v1/chat/completions" {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id":      "chatcmpl-test",
				"choices": []map[string]any{{"message": map[string]any{"role": "assistant", "content": "pong"}}},
				"usage":   map[string]any{"prompt_tokens": 5, "completion_tokens": 5, "total_tokens": 10},
			})
			return
		}
		http.NotFound(w, r)
	}))
	defer upstream.Close()

	st := newTestStore(t)
	dsAcc, err := st.CreateAccount(&store.Account{
		AccountType: store.AccountTypeAPIKey,
		BaseURL:     upstream.URL,
		APIKey:      "sk-test",
		Status:      store.AccountActive,
	})
	if err != nil {
		t.Fatal(err)
	}

	e := gateway.New(st, account.NewManager(st), func() store.Settings { s, _ := st.LoadSettings(); return s }, nil)

	// 1. Probing models for the account
	res, err := e.AccountAvailableModels(t.Context(), dsAcc)
	if err != nil {
		t.Fatalf("AccountAvailableModels failed: %v", err)
	}
	if res.Source != "upstream" {
		t.Fatalf("res.Source = %q, want 'upstream'", res.Source)
	}
	if len(res.Models) != 3 {
		t.Fatalf("expected 3 models, got %d: %v", len(res.Models), res.Models)
	}
	// Verify deepseek-chat is prioritized first
	if res.Models[0] != "deepseek-chat" {
		t.Fatalf("expected deepseek-chat to be first, got: %s", res.Models[0])
	}
	if res.DefaultModel != "deepseek-chat" {
		t.Fatalf("expected default model deepseek-chat, got: %s", res.DefaultModel)
	}

	// 2. ActiveModels should now include the dynamic models from the cache
	active := e.ActiveModels()
	foundChat := false
	for _, m := range active {
		if m == "deepseek-chat" {
			foundChat = true
		}
	}
	if !foundChat {
		t.Fatalf("ActiveModels() should include deepseek-chat from cache, got: %v", active)
	}

	// 3. TestAccount fallback: user tests with gpt-5.6-sol on this deepseek account
	// TestAccount should adapt and succeed with deepseek-chat instead of failing
	testRes := e.TestAccount(t.Context(), dsAcc, "gpt-5.6-sol", "hi")
	if !testRes.OK {
		t.Fatalf("TestAccount failed: %+v", testRes)
	}
	if testRes.Model != "deepseek-chat" {
		t.Fatalf("TestAccount should adapt model to deepseek-chat, got: %s", testRes.Model)
	}

	// 4. Test ProbeModels for ad-hoc base_url + key
	probeRes, err := e.ProbeModels(t.Context(), upstream.URL, "sk-test", nil, store.AccountNetworkDirect)
	if err != nil {
		t.Fatalf("ProbeModels failed: %v", err)
	}
	if len(probeRes.Models) != 3 || probeRes.Source != "upstream" {
		t.Fatalf("unexpected probeRes: %+v", probeRes)
	}
}
