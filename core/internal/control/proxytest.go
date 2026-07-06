package control

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"sub2api-desktop/core/internal/gateway"
	"sub2api-desktop/core/internal/store"
)

// testProxyLatency issues a lightweight request through the proxy and measures
// round-trip latency.
func testProxyLatency(ctx context.Context, proxy *store.Proxy) (time.Duration, error) {
	client, err := gateway.NewAuthClient(proxy)
	if err != nil {
		return 0, err
	}
	client.Timeout = 15 * time.Second

	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://chatgpt.com/cdn-cgi/trace", nil)
	if err != nil {
		return 0, err
	}
	start := time.Now()
	resp, err := client.Do(req)
	if err != nil {
		return 0, fmt.Errorf("代理连接失败: %w", err)
	}
	defer resp.Body.Close()
	latency := time.Since(start)
	if resp.StatusCode >= 500 {
		return latency, fmt.Errorf("代理可连接但目标返回 %d", resp.StatusCode)
	}
	return latency, nil
}
