package cloudsync

import (
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"sub2api-desktop/core/internal/store"
)

type NetworkInfo struct {
	Mode          store.CloudConnectionMode `json:"mode"`
	ProxyID       *int64                    `json:"proxy_id,omitempty"`
	ProxyName     string                    `json:"proxy_name,omitempty"`
	ProxyType     string                    `json:"proxy_type,omitempty"`
	EffectiveFrom string                    `json:"effective_source"`
}

func newDefaultCloudHTTPClient() *http.Client {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.Proxy = cloudProxy
	return &http.Client{Transport: transport, Timeout: 30 * time.Second}
}

func newConfiguredCloudHTTPClient(baseURL string, value store.CloudConnectionSettings, selected *store.Proxy) (*http.Client, NetworkInfo, error) {
	info := NetworkInfo{Mode: value.Mode, ProxyID: value.ProxyID}
	transport := http.DefaultTransport.(*http.Transport).Clone()
	switch value.Mode {
	case store.CloudConnectionDirect:
		transport.Proxy = nil
		info.EffectiveFrom = "direct"
	case store.CloudConnectionSystem:
		transport.Proxy = cloudProxy
		info.EffectiveFrom = systemProxySource(baseURL)
	case store.CloudConnectionProxy:
		if selected == nil || value.ProxyID == nil || selected.ID != *value.ProxyID {
			return nil, info, errors.New("selected Amber Cloud proxy is unavailable")
		}
		proxyURL, err := storedProxyURL(selected)
		if err != nil {
			return nil, info, err
		}
		transport.Proxy = http.ProxyURL(proxyURL)
		info.ProxyName = selected.Name
		info.ProxyType = string(selected.Type)
		info.EffectiveFrom = "amber_proxy"
	default:
		return nil, info, fmt.Errorf("unsupported Amber Cloud connection mode %q", value.Mode)
	}
	return &http.Client{Transport: transport, Timeout: 30 * time.Second}, info, nil
}

func storedProxyURL(proxy *store.Proxy) (*url.URL, error) {
	if proxy == nil || strings.TrimSpace(proxy.Host) == "" || proxy.Port < 1 || proxy.Port > 65535 {
		return nil, errors.New("selected Amber Cloud proxy is invalid")
	}
	scheme := strings.ToLower(strings.TrimSpace(string(proxy.Type)))
	if scheme != "http" && scheme != "https" && scheme != "socks5" {
		return nil, errors.New("selected Amber Cloud proxy type is unsupported")
	}
	result := &url.URL{Scheme: scheme, Host: net.JoinHostPort(strings.TrimSpace(proxy.Host), strconv.Itoa(proxy.Port))}
	if proxy.Username != "" {
		if proxy.Password != "" {
			result.User = url.UserPassword(proxy.Username, proxy.Password)
		} else {
			result.User = url.User(proxy.Username)
		}
	}
	return result, nil
}

func systemProxySource(baseURL string) string {
	request, err := http.NewRequest(http.MethodGet, strings.TrimRight(baseURL, "/")+"/health", nil)
	if err == nil {
		if proxy, proxyErr := http.ProxyFromEnvironment(request); proxyErr == nil && proxy != nil {
			return "environment"
		}
	}
	if parseStaticProxy(systemProxyAddress(), "https") != nil {
		return "windows"
	}
	return "direct"
}

func cloudProxy(request *http.Request) (*url.URL, error) {
	proxy, err := http.ProxyFromEnvironment(request)
	if proxy != nil || err != nil {
		return proxy, err
	}
	return parseStaticProxy(systemProxyAddress(), request.URL.Scheme), nil
}

func parseStaticProxy(value, scheme string) *url.URL {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	selected := value
	if strings.Contains(value, ";") || strings.Contains(value, "=") {
		selected = ""
		fallback := ""
		for _, entry := range strings.Split(value, ";") {
			key, address, found := strings.Cut(strings.TrimSpace(entry), "=")
			if !found {
				if fallback == "" {
					fallback = strings.TrimSpace(entry)
				}
				continue
			}
			if strings.EqualFold(strings.TrimSpace(key), scheme) {
				selected = strings.TrimSpace(address)
				break
			}
			if fallback == "" && (strings.EqualFold(strings.TrimSpace(key), "http") || strings.EqualFold(strings.TrimSpace(key), "https")) {
				fallback = strings.TrimSpace(address)
			}
		}
		if selected == "" {
			selected = fallback
		}
	}
	if selected == "" {
		return nil
	}
	if !strings.Contains(selected, "://") {
		selected = "http://" + selected
	}
	parsed, err := url.Parse(selected)
	if err != nil || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https" && parsed.Scheme != "socks5") {
		return nil
	}
	return parsed
}
