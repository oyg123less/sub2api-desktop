package store

import (
	"testing"
	"time"
)

func TestSummaryExcludesClientCancellationFromSuccessRateDenominator(t *testing.T) {
	st := openCloudTestStore(t)
	logs := []*RequestLog{
		{StatusCode: 200, PromptTokens: 10, CachedTokens: 3, CompletionTokens: 5, ReasoningTokens: 2},
		{StatusCode: 502, ErrorKind: "upstream_stream_error"},
		{StatusCode: 499, ErrorKind: "client_cancelled", PromptTokens: 8, CompletionTokens: 2, Estimated: true},
	}
	for _, entry := range logs {
		if err := st.InsertLog(entry); err != nil {
			t.Fatal(err)
		}
	}
	summary, err := st.Summary(time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	if summary.TotalRequests != 3 || summary.EligibleRequests != 2 || summary.SuccessRequests != 1 || summary.FailedRequests != 1 || summary.ClientCancelled != 1 {
		t.Fatalf("unexpected request summary: %+v", summary)
	}
	if summary.CachedTokens != 3 || summary.ReasoningTokens != 2 || summary.EstimatedRequests != 1 {
		t.Fatalf("unexpected usage summary: %+v", summary)
	}
	failures, err := st.FailureBreakdown(time.Time{})
	if err != nil || len(failures) != 1 || failures[0].Kind != "stream_interrupted" || failures[0].Requests != 1 {
		t.Fatalf("failures=%#v err=%v", failures, err)
	}
}
