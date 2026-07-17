package cloudsync

import (
	"net/http"
	"net/url"
	"strings"
	"time"
)

func newDefaultCloudHTTPClient() *http.Client {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.Proxy = cloudProxy
	return &http.Client{Transport: transport, Timeout: 30 * time.Second}
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
