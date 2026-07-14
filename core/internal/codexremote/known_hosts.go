package codexremote

import (
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"sync"

	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/knownhosts"
)

type knownHostStore struct {
	path string
	mu   sync.Mutex
}

type hostKeyCapture struct {
	fingerprint string
	known       bool
	key         ssh.PublicKey
}

func newKnownHostStore(path string) (*knownHostStore, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, err
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND, 0o600)
	if err != nil {
		return nil, err
	}
	if err := file.Close(); err != nil {
		return nil, err
	}
	return &knownHostStore{path: path}, nil
}

func (s *knownHostStore) callback(targetAddress string, allowUnknown bool, capture *hostKeyCapture) (ssh.HostKeyCallback, error) {
	s.mu.Lock()
	callback, err := knownhosts.New(s.path)
	s.mu.Unlock()
	if err != nil {
		return nil, err
	}
	return func(_ string, remote net.Addr, key ssh.PublicKey) error {
		capture.fingerprint = ssh.FingerprintSHA256(key)
		capture.key = key
		err := callback(targetAddress, remote, key)
		if err == nil {
			capture.known = true
			return nil
		}
		var keyError *knownhosts.KeyError
		if !errors.As(err, &keyError) {
			return codedError("connection_failed", err)
		}
		if len(keyError.Want) > 0 {
			return &Error{Code: "host_key_mismatch", Fingerprint: capture.fingerprint, cause: err}
		}
		if !allowUnknown {
			return &Error{Code: "host_key_unknown", Fingerprint: capture.fingerprint, cause: err}
		}
		return nil
	}, nil
}

func (s *knownHostStore) trust(targetAddress string, key ssh.PublicKey) error {
	if key == nil {
		return fmt.Errorf("host key is missing")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	callback, err := knownhosts.New(s.path)
	if err != nil {
		return err
	}
	if err := callback(targetAddress, dummyAddr(targetAddress), key); err == nil {
		return nil
	} else {
		var keyError *knownhosts.KeyError
		if errors.As(err, &keyError) && len(keyError.Want) > 0 {
			return codedError("host_key_mismatch", err)
		}
	}
	file, err := os.OpenFile(s.path, os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	_, writeErr := fmt.Fprintln(file, knownhosts.Line([]string{knownhosts.Normalize(targetAddress)}, key))
	closeErr := file.Close()
	if writeErr != nil {
		return writeErr
	}
	return closeErr
}

type stringAddr string

func (a stringAddr) Network() string  { return "tcp" }
func (a stringAddr) String() string   { return string(a) }
func dummyAddr(value string) net.Addr { return stringAddr(value) }
