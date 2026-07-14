package codexremote

import (
	"crypto/rand"
	"crypto/rsa"
	"errors"
	"net"
	"path/filepath"
	"testing"

	"golang.org/x/crypto/ssh"
)

func TestKnownHostsTOFUAndMismatch(t *testing.T) {
	hosts, err := newKnownHostStore(filepath.Join(t.TempDir(), "known_hosts"))
	if err != nil {
		t.Fatal(err)
	}
	first := testPublicKey(t)
	second := testPublicKey(t)
	target := "example.test:22"

	capture := &hostKeyCapture{}
	callback, err := hosts.callback(target, true, capture)
	if err != nil {
		t.Fatal(err)
	}
	if err := callback(target, testAddr(target), first); err != nil {
		t.Fatal(err)
	}
	if capture.known || capture.fingerprint != ssh.FingerprintSHA256(first) {
		t.Fatal("first-use host key was not captured as unknown")
	}
	if err := hosts.trust(target, first); err != nil {
		t.Fatal(err)
	}

	capture = &hostKeyCapture{}
	callback, err = hosts.callback(target, false, capture)
	if err != nil {
		t.Fatal(err)
	}
	if err := callback(target, testAddr(target), first); err != nil || !capture.known {
		t.Fatal("trusted host key was not accepted")
	}
	err = callback(target, testAddr(target), second)
	var remoteError *Error
	if !errors.As(err, &remoteError) || remoteError.Code != "host_key_mismatch" {
		t.Fatalf("mismatched host key error = %v", err)
	}
}

func testPublicKey(t *testing.T) ssh.PublicKey {
	t.Helper()
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	key, err := ssh.NewPublicKey(&privateKey.PublicKey)
	if err != nil {
		t.Fatal(err)
	}
	return key
}

type testAddr string

func (a testAddr) Network() string { return "tcp" }
func (a testAddr) String() string  { return string(a) }

var _ net.Addr = testAddr("")
