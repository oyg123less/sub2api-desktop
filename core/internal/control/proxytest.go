package control

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"sub2api-desktop/core/internal/store"
	apptransport "sub2api-desktop/core/internal/transport"
)

const (
	proxyTestTarget         = "https://chatgpt.com/cdn-cgi/trace"
	proxyTestAttempts       = 2
	proxyTestAttemptTimeout = 10 * time.Second
	proxyTestTotalTimeout   = 22 * time.Second
)

type proxyTestStage struct {
	ID     string `json:"id"`
	Status string `json:"status"`
}

type proxyTestResult struct {
	OK        bool             `json:"ok"`
	LatencyMS int64            `json:"latency_ms,omitempty"`
	ErrorKind string           `json:"error_kind,omitempty"`
	Error     string           `json:"error,omitempty"`
	Stages    []proxyTestStage `json:"stages"`
}

// testProxyLatency issues a lightweight request through the proxy and measures
// round-trip latency.
func testProxyLatency(ctx context.Context, proxy *store.Proxy) proxyTestResult {
	return testProxyLatencyTo(ctx, proxy, proxyTestTarget)
}

func testProxyLatencyTo(ctx context.Context, proxy *store.Proxy, target string) proxyTestResult {
	client, err := apptransport.NewClient(apptransport.Options{
		Proxy:   proxy,
		Purpose: apptransport.PurposeProxyTest,
		// A proxy test verifies the proxy, independently from the gateway's
		// optional Codex fingerprint profile.
		FingerprintProfile: "standard",
		Timeout:            proxyTestAttemptTimeout,
	})
	if err != nil {
		return failedProxyTest(proxy, apptransport.Kind(err), err)
	}
	client.Timeout = proxyTestAttemptTimeout

	ctx, cancel := context.WithTimeout(ctx, proxyTestTotalTimeout)
	defer cancel()

	start := time.Now()
	var lastErr error
	for attempt := 0; attempt < proxyTestAttempts; attempt++ {
		attemptCtx, attemptCancel := context.WithTimeout(ctx, proxyTestAttemptTimeout)
		req, requestErr := http.NewRequestWithContext(attemptCtx, http.MethodGet, target, nil)
		if requestErr != nil {
			attemptCancel()
			return failedProxyTest(proxy, apptransport.ErrorTargetHTTP, requestErr)
		}
		resp, requestErr := client.Do(req)
		attemptCancel()
		if requestErr == nil {
			_ = resp.Body.Close()
			if resp.StatusCode < http.StatusInternalServerError {
				return proxyTestResult{OK: true, LatencyMS: time.Since(start).Milliseconds(), Stages: proxyStages(proxy, "")}
			}
			lastErr = fmt.Errorf("target returned HTTP %d", resp.StatusCode)
		} else {
			lastErr = requestErr
			if !apptransport.IsTransientNetworkError(requestErr) {
				break
			}
		}
		client.CloseIdleConnections()
	}
	return failedProxyTest(proxy, apptransport.Kind(lastErr), lastErr)
}

func failedProxyTest(proxy *store.Proxy, kind apptransport.ErrorKind, err error) proxyTestResult {
	return proxyTestResult{OK: false, ErrorKind: string(kind), Error: err.Error(), Stages: proxyStages(proxy, kind)}
}

func proxyStages(proxy *store.Proxy, failed apptransport.ErrorKind) []proxyTestStage {
	ids := []string{"proxy_connect", "proxy_auth", "proxy_tls", "target_tls", "target_http"}
	stages := make([]proxyTestStage, 0, len(ids))
	failureIndex := len(ids)
	for i, id := range ids {
		if id == string(failed) {
			failureIndex = i
		}
	}
	for i, id := range ids {
		status := "ok"
		if (id == "proxy_auth" && proxy.Username == "") || (id == "proxy_tls" && proxy.Type != store.ProxyHTTPS) {
			status = "skipped"
		}
		if i == failureIndex {
			status = "failed"
		} else if i > failureIndex {
			status = "not_run"
		}
		stages = append(stages, proxyTestStage{ID: id, Status: status})
	}
	return stages
}
