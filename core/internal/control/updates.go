package control

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"sub2api-desktop/core/internal/store"
	apptransport "sub2api-desktop/core/internal/transport"
)

const (
	githubLatestReleaseURL = "https://api.github.com/repos/oyg123less/sub2api-desktop/releases/latest"
	updateCacheTTL         = 6 * time.Hour
	updateRequestTimeout   = 15 * time.Second
)

type releaseInfo struct {
	TagName     string    `json:"tag_name"`
	Name        string    `json:"name"`
	Body        string    `json:"body"`
	HTMLURL     string    `json:"html_url"`
	PublishedAt string    `json:"published_at"`
	CheckedAt   time.Time `json:"checked_at"`
}

type updateChecker struct {
	mu          sync.Mutex
	settings    SettingsAccess
	listProxies func() ([]*store.Proxy, error)
	version     string
	latestURL   string
	cached      *releaseInfo
	cachedAt    time.Time
}

func newUpdateChecker(s *store.Store, settings SettingsAccess, version string) *updateChecker {
	return &updateChecker{
		settings:    settings,
		listProxies: s.ListProxies,
		version:     version,
		latestURL:   githubLatestReleaseURL,
	}
}

func (c *Control) latestRelease(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), updateRequestTimeout)
	defer cancel()
	release, err := c.updates.latest(ctx)
	if err != nil {
		writeControlError(w, http.StatusBadGateway, "update_check_failed", err.Error(), true, nil)
		return
	}
	writeJSON(w, http.StatusOK, release)
}

func (u *updateChecker) latest(ctx context.Context) (*releaseInfo, error) {
	u.mu.Lock()
	defer u.mu.Unlock()
	if u.cached != nil && time.Since(u.cachedAt) < updateCacheTTL {
		copy := *u.cached
		return &copy, nil
	}

	release, err := u.fetchLatest(ctx)
	if err != nil {
		return nil, err
	}
	release.CheckedAt = time.Now().UTC()
	u.cached = release
	u.cachedAt = time.Now()
	copy := *release
	return &copy, nil
}

func (u *updateChecker) fetchLatest(ctx context.Context) (*releaseInfo, error) {
	proxies, _ := u.listProxies()
	attempts := append([]*store.Proxy(nil), proxies...)
	attempts = append(attempts, nil)
	var lastErr error
	for _, proxy := range attempts {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		client, err := apptransport.NewClient(apptransport.Options{
			Proxy: proxy, Purpose: apptransport.PurposeDiagnostic, FingerprintProfile: "standard", Timeout: 10 * time.Second,
		})
		if err != nil {
			lastErr = err
			continue
		}
		release, err := u.requestLatest(ctx, client)
		client.CloseIdleConnections()
		if err == nil {
			return release, nil
		}
		lastErr = err
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("no update request route is available")
	}
	return nil, lastErr
}

func (u *updateChecker) requestLatest(ctx context.Context, client *http.Client) (*releaseInfo, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.latestURL, nil)
	if err != nil {
		return nil, err
	}
	userAgent := strings.TrimSpace(u.settings.Get().UserAgent)
	if userAgent == "" {
		userAgent = "Amber/" + strings.TrimSpace(u.version)
	}
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")

	response, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(response.Body, 4<<10))
		return nil, fmt.Errorf("GitHub releases returned HTTP %d: %s", response.StatusCode, strings.TrimSpace(string(body)))
	}
	var release releaseInfo
	if err := json.NewDecoder(io.LimitReader(response.Body, 2<<20)).Decode(&release); err != nil {
		return nil, fmt.Errorf("decode GitHub release: %w", err)
	}
	if strings.TrimSpace(release.TagName) == "" || strings.TrimSpace(release.HTMLURL) == "" {
		return nil, fmt.Errorf("GitHub release is missing tag_name or html_url")
	}
	return &release, nil
}
