package apicompat

import (
	"encoding/json"
	"reflect"
	"testing"
)

func TestChatCompletionsToResponsesToolChoice(t *testing.T) {
	tests := []struct {
		name        string
		choice      string
		want        any
		wantRawSame bool
	}{
		{name: "auto", choice: `"auto"`, want: "auto", wantRawSame: true},
		{name: "none", choice: `"none"`, want: "none", wantRawSame: true},
		{name: "required", choice: `"required"`, want: "required", wantRawSame: true},
		{
			name:   "chat named function",
			choice: `{"type":"function","function":{"name":"lookup"}}`,
			want:   map[string]any{"type": "function", "name": "lookup"},
		},
		{
			name:        "responses named function",
			choice:      `{"type":"function", "name":"lookup"}`,
			want:        map[string]any{"type": "function", "name": "lookup"},
			wantRawSame: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			request := conversionTestRequest()
			request.ToolChoice = json.RawMessage(tt.choice)
			response, err := ChatCompletionsToResponses(request)
			if err != nil {
				t.Fatal(err)
			}
			if tt.wantRawSame && string(response.ToolChoice) != tt.choice {
				t.Fatalf("tool_choice raw value = %s, want %s", response.ToolChoice, tt.choice)
			}
			var got any
			if err := json.Unmarshal(response.ToolChoice, &got); err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("tool_choice = %#v, want %#v", got, tt.want)
			}
		})
	}
}

func TestChatCompletionsToResponsesRejectsInvalidToolChoiceObjects(t *testing.T) {
	for _, choice := range []string{
		`{}`,
		`{"type":"function"}`,
		`{"type":"function","function":{}}`,
		`{"type":"other","name":"lookup"}`,
		`[]`,
	} {
		t.Run(choice, func(t *testing.T) {
			request := conversionTestRequest()
			request.ToolChoice = json.RawMessage(choice)
			if _, err := ChatCompletionsToResponses(request); err == nil {
				t.Fatalf("invalid tool_choice %s was accepted", choice)
			}
		})
	}
}

func TestChatCompletionsToResponsesSupportsDeveloperRole(t *testing.T) {
	request := &ChatCompletionsRequest{
		Model: "gpt-5.4",
		Messages: []ChatMessage{
			{Role: "developer", Content: json.RawMessage(`"follow project rules"`)},
			{Role: "future-role", Content: json.RawMessage(`"remain tolerant"`)},
		},
	}
	response, err := ChatCompletionsToResponses(request)
	if err != nil {
		t.Fatal(err)
	}
	var input []ResponsesInputItem
	if err := json.Unmarshal(response.Input, &input); err != nil {
		t.Fatal(err)
	}
	if len(input) != 2 {
		t.Fatalf("input item count = %d, want 2", len(input))
	}
	if input[0].Role != "developer" || string(input[0].Content) != `"follow project rules"` {
		t.Fatalf("developer item = %#v", input[0])
	}
	if response.Instructions != "follow project rules" {
		t.Fatalf("instructions = %q, want developer text", response.Instructions)
	}
	if input[1].Role != "user" || string(input[1].Content) != `"remain tolerant"` {
		t.Fatalf("unknown role did not retain user fallback: %#v", input[1])
	}
}

func conversionTestRequest() *ChatCompletionsRequest {
	return &ChatCompletionsRequest{
		Model: "gpt-5.4",
		Messages: []ChatMessage{{
			Role:    "user",
			Content: json.RawMessage(`"hello"`),
		}},
	}
}
