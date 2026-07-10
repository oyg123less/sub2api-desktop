package gateway

import (
	"net/http"
	"time"

	"sub2api-desktop/core/internal/store"
	apptransport "sub2api-desktop/core/internal/transport"
)

func newHTTPClient(proxy *store.Proxy, profile string, timeout time.Duration) (*http.Client, error) {
	if profile == "" {
		profile = "standard"
	}
	return apptransport.NewClient(apptransport.Options{
		Proxy: proxy, Purpose: apptransport.PurposeGateway, FingerprintProfile: profile, Timeout: timeout,
	})
}
