//go:build !windows

package cloudsync

func systemProxyAddress() string { return "" }
