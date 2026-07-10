package gateway

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	"sub2api-desktop/core/internal/apicompat"
)

type sseTerminal struct {
	event     string
	completed bool
	status    int
	errorKind string
	message   string
	response  *apicompat.ResponsesResponse
	usage     *apicompat.ResponsesUsage
}

func (s *sseTerminal) observe(evt *apicompat.ResponsesStreamEvent) {
	if evt == nil {
		return
	}
	if evt.Usage != nil {
		s.usage = evt.Usage
	}
	if evt.Response != nil {
		s.response = evt.Response
		if evt.Response.Usage != nil {
			s.usage = evt.Response.Usage
		}
	}

	switch evt.Type {
	case "response.completed", "response.done":
		if evt.Response != nil && evt.Response.Status != "" && evt.Response.Status != "completed" {
			s.fail(evt.Type, "upstream_failed_event", responseFailureMessage(evt.Response))
			return
		}
		s.event = evt.Type
		s.completed = true
	case "response.incomplete":
		reason := ""
		if evt.Response != nil && evt.Response.IncompleteDetails != nil {
			reason = evt.Response.IncompleteDetails.Reason
		}
		if reason == "max_output_tokens" {
			s.event = evt.Type
			s.completed = true
			return
		}
		message := "upstream response was incomplete"
		if reason != "" {
			message += ": " + reason
		}
		s.fail(evt.Type, "upstream_failed_event", message)
	case "response.failed":
		s.fail(evt.Type, "upstream_failed_event", responseFailureMessage(evt.Response))
	case "error":
		message := strings.TrimSpace(evt.Message)
		if message == "" {
			message = strings.TrimSpace(evt.Code)
		}
		if message == "" {
			message = "upstream emitted an error event"
		}
		s.fail(evt.Type, "upstream_failed_event", message)
	}
}

func (s *sseTerminal) fail(event, kind, message string) {
	s.event = event
	s.completed = false
	s.status = http.StatusBadGateway
	s.errorKind = kind
	s.message = message
}

func (s *sseTerminal) finish(scanErr error) error {
	if scanErr != nil {
		s.fail("scanner_error", "upstream_stream_error", "upstream stream interrupted: "+scanErr.Error())
	}
	if s.errorKind != "" {
		return errors.New(s.message)
	}
	if !s.completed {
		s.fail("missing_terminal", "upstream_stream_error", "upstream stream ended without a terminal event")
		return errors.New(s.message)
	}
	return nil
}

func responseFailureMessage(resp *apicompat.ResponsesResponse) string {
	if resp != nil {
		if resp.Error != nil && strings.TrimSpace(resp.Error.Message) != "" {
			return strings.TrimSpace(resp.Error.Message)
		}
		if resp.IncompleteDetails != nil && strings.TrimSpace(resp.IncompleteDetails.Reason) != "" {
			return "upstream response failed: " + resp.IncompleteDetails.Reason
		}
		if strings.TrimSpace(resp.Status) != "" {
			return "upstream response status: " + resp.Status
		}
	}
	return "upstream response failed"
}

func writeStreamError(w io.Writer, kind, message string) {
	payload, _ := json.Marshal(apiError{Error: apiErrorBody{
		Message: message,
		Type:    "upstream_error",
		Code:    kind,
	}})
	_, _ = fmt.Fprintf(w, "data: %s\n\n", payload)
}
