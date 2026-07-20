package store

import (
	"errors"
	"testing"
)

func TestAccountNetworkModesAreExplicitAndValidated(t *testing.T) {
	st := openCloudTestStore(t)
	account := createBatchTestAccount(t, st, "network", "sk-network")
	proxy, err := st.CreateProxy(&Proxy{Name: "network proxy", Type: ProxyHTTP, Host: "127.0.0.1", Port: 7890})
	if err != nil {
		t.Fatal(err)
	}

	if err := st.SetAccountNetwork(account.ID, AccountNetworkSystem, nil); err != nil {
		t.Fatal(err)
	}
	account, err = st.GetAccount(account.ID)
	if err != nil || account.NetworkMode != AccountNetworkSystem || account.ProxyID != nil {
		t.Fatalf("system route was not saved: %+v err=%v", account, err)
	}

	if err := st.SetAccountNetwork(account.ID, AccountNetworkProxy, &proxy.ID); err != nil {
		t.Fatal(err)
	}
	account, err = st.GetAccount(account.ID)
	if err != nil || account.NetworkMode != AccountNetworkProxy || account.ProxyID == nil || *account.ProxyID != proxy.ID {
		t.Fatalf("proxy route was not saved: %+v err=%v", account, err)
	}

	if err := st.SetAccountNetwork(account.ID, AccountNetworkSystem, &proxy.ID); err == nil {
		t.Fatal("system mode accepted an Amber proxy")
	}
	missing := int64(999999)
	if err := st.SetAccountNetwork(account.ID, AccountNetworkProxy, &missing); !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing proxy error = %v, want ErrNotFound", err)
	}

	if err := st.DeleteProxy(proxy.ID); err != nil {
		t.Fatal(err)
	}
	account, err = st.GetAccount(account.ID)
	if err != nil || account.NetworkMode != AccountNetworkDirect || account.ProxyID != nil {
		t.Fatalf("deleted proxy did not fall back to direct: %+v err=%v", account, err)
	}
}
