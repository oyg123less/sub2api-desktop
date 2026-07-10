package apiserver

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestModelsAdvertisesGPT56(t *testing.T) {
	handler := &Handler{}
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/v1/models", nil)
	handler.models(recorder, request)

	var response struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	wanted := map[string]bool{
		"gpt-5.6-sol":   false,
		"gpt-5.6-terra": false,
		"gpt-5.6-luna":  false,
	}
	for _, model := range response.Data {
		if _, exists := wanted[model.ID]; exists {
			wanted[model.ID] = true
		}
		if model.ID == "gpt-image-2" {
			t.Fatal("image model was advertised by the text gateway")
		}
	}
	for model, found := range wanted {
		if !found {
			t.Fatalf("/v1/models missing %q", model)
		}
	}
}
