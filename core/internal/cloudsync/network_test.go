package cloudsync

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"sub2api-desktop/core/internal/store"
)

func TestNetworkSettingsSwitchWithoutRestartAndProbeHealth(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/health" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer server.Close()
	st := openTestStore(t, "network")
	manager := NewManager(st, &testSettings{value: store.DefaultSettings()}, server.URL, "site-key", nil, nil)
	settings, err := manager.UpdateNetworkSettings(store.CloudConnectionSettings{Mode: store.CloudConnectionDirect})
	if err != nil {
		t.Fatal(err)
	}
	if settings.Mode != store.CloudConnectionDirect || settings.EffectiveFrom != "direct" {
		t.Fatalf("unexpected settings: %#v", settings)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	probe := manager.ProbeNetwork(ctx)
	if !probe.OK || len(probe.Stages) != 4 || probe.Stages[3].ID != "http" || probe.Stages[3].Status != "ok" {
		t.Fatalf("unexpected probe: %#v", probe)
	}
}

func TestSelectedCloudProxyIsSanitizedAndFailsClosedAfterDeletion(t *testing.T) {
	st := openTestStore(t, "network-proxy")
	manager := NewManager(st, &testSettings{value: store.DefaultSettings()}, "https://cloud.example.test", "site-key", nil, nil)
	proxy, err := st.CreateProxy(&store.Proxy{
		Name: "private relay", Type: store.ProxySOCKS5, Host: "127.0.0.1", Port: 1080,
		Username: "secret-user", Password: "secret-password",
	})
	if err != nil {
		t.Fatal(err)
	}
	settings, err := manager.UpdateNetworkSettings(store.CloudConnectionSettings{Mode: store.CloudConnectionProxy, ProxyID: &proxy.ID})
	if err != nil {
		t.Fatal(err)
	}
	if settings.ProxyName != proxy.Name || settings.ProxyType != string(proxy.Type) || settings.EffectiveFrom != "amber_proxy" {
		t.Fatalf("unexpected sanitized settings: %#v", settings)
	}
	relayProxy, err := manager.relayProxyURL()
	if err != nil {
		t.Fatal(err)
	}
	if relayProxy.Scheme != "socks5" || relayProxy.Host != "127.0.0.1:1080" {
		t.Fatalf("unexpected relay proxy: %s", relayProxy)
	}
	if err := st.DeleteProxy(proxy.ID); err != nil {
		t.Fatal(err)
	}
	if err := manager.ReloadNetworkSettings(); err == nil {
		t.Fatal("deleted selected proxy did not fail closed")
	}
	if _, err := manager.client.httpClient(); err == nil {
		t.Fatal("cloud client remained usable after selected proxy deletion")
	}
}
