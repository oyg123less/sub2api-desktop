package diagnostics

import (
	"io"
	"net/http"
	"path/filepath"
	"strings"
	"testing"
	"time"

	appcrypto "sub2api-desktop/core/internal/crypto"
	"sub2api-desktop/core/internal/store"
)

type fakeServer struct{}

func (fakeServer) Running() bool { return false }
func (fakeServer) Port() int     { return 8080 }

type roundTripFunc func(*http.Request) (*http.Response, error)

func (fn roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) { return fn(request) }

func TestRunCompletesWithoutAccountsOrProxies(t *testing.T) {
	dir := t.TempDir()
	cipher, err := appcrypto.LoadOrCreate(filepath.Join(dir, "key"))
	if err != nil {
		t.Fatal(err)
	}
	st, err := store.Open(filepath.Join(dir, "sub2api.db"), cipher)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	service := New(st, store.DefaultSettings, fakeServer{}, dir, "0.2.0")
	service.directHTTP = &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader("ok")), Header: make(http.Header)}, nil
	})}
	run := service.Start()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		run, err = service.Get(run.RunID)
		if err != nil {
			t.Fatal(err)
		}
		if run.Status == "completed" {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if run.Status != "completed" || run.Progress != 100 {
		t.Fatalf("diagnostic run did not complete: %#v", run)
	}
	if len(run.Checks) != 12 {
		t.Fatalf("check count = %d, want 12", len(run.Checks))
	}
	if run.Summary.Warning == 0 {
		t.Fatal("empty account pool should produce a warning")
	}
}

func TestReportIsSanitized(t *testing.T) {
	service := &Service{runs: map[string]*storedRun{}, order: []string{}}
	stored := &storedRun{run: Run{
		RunID: "diag_test", Status: "completed", CreatedAt: time.Now(),
		Checks: []Check{{ID: "secret", Status: StatusFailed, Title: "Secret", Message: "token sk-secret-value-123456789 and user a.person@example.com"}},
	}}
	service.runs["diag_test"] = stored
	data, _, err := service.Report("diag_test", "json")
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	if strings.Contains(text, "sk-secret-value") || strings.Contains(text, "a.person@example.com") {
		t.Fatalf("report contains sensitive data: %s", text)
	}
}
