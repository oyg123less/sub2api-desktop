package codexremote

import (
	"bytes"
	"context"
	"errors"
	"net"
	"strings"
	"time"

	"golang.org/x/crypto/ssh"
)

type sshConnection struct{ client *ssh.Client }

func (c *sshConnection) Run(ctx context.Context, command string, input []byte) ([]byte, error) {
	session, err := c.client.NewSession()
	if err != nil {
		return nil, err
	}
	defer session.Close()
	session.Stdin = bytes.NewReader(input)
	var output bytes.Buffer
	session.Stdout = &output
	session.Stderr = &output
	if err := session.Start(command); err != nil {
		return nil, err
	}
	done := make(chan error, 1)
	go func() { done <- session.Wait() }()
	select {
	case <-ctx.Done():
		_ = session.Close()
		return nil, ctx.Err()
	case err := <-done:
		return output.Bytes(), err
	}
}

func (c *sshConnection) Listen(network, address string) (net.Listener, error) {
	return c.client.Listen(network, address)
}

func (c *sshConnection) SendRequest(name string, wantReply bool, payload []byte) (bool, []byte, error) {
	return c.client.SendRequest(name, wantReply, payload)
}

func (c *sshConnection) Close() error { return c.client.Close() }

func makeSSHDialer(hosts *knownHostStore) dialFunc {
	return func(ctx context.Context, target ProbeRequest, allowUnknown, trustUnknown bool) (*dialResult, error) {
		target, err := normalizeProbeRequest(target)
		if err != nil {
			return nil, err
		}
		targetAddress := address(target.Host, target.Port)
		capture := &hostKeyCapture{}
		callback, err := hosts.callback(targetAddress, allowUnknown, capture)
		if err != nil {
			return nil, codedError("connection_failed", err)
		}
		config := &ssh.ClientConfig{
			User: target.User, Auth: []ssh.AuthMethod{ssh.Password(target.Password)},
			HostKeyCallback: callback, Timeout: 10 * time.Second,
		}
		dialer := net.Dialer{Timeout: 10 * time.Second}
		netConn, err := dialer.DialContext(ctx, "tcp", targetAddress)
		if err != nil {
			return nil, codedError("connection_failed", err)
		}
		_ = netConn.SetDeadline(time.Now().Add(10 * time.Second))
		clientConn, channels, requests, err := ssh.NewClientConn(netConn, targetAddress, config)
		if err != nil {
			_ = netConn.Close()
			var coded *Error
			if errors.As(err, &coded) {
				return nil, coded
			}
			if strings.Contains(strings.ToLower(err.Error()), "authenticate") {
				return nil, codedError("auth_failed", err)
			}
			return nil, codedError("connection_failed", err)
		}
		_ = netConn.SetDeadline(time.Time{})
		client := ssh.NewClient(clientConn, channels, requests)
		if trustUnknown && !capture.known {
			if err := hosts.trust(targetAddress, capture.key); err != nil {
				_ = client.Close()
				return nil, err
			}
			capture.known = true
		}
		return &dialResult{connection: &sshConnection{client: client}, fingerprint: capture.fingerprint, known: capture.known}, nil
	}
}

func normalizeProbeRequest(target ProbeRequest) (ProbeRequest, error) {
	target.Host = strings.TrimSpace(target.Host)
	target.User = strings.TrimSpace(target.User)
	if user, host, ok := strings.Cut(target.Host, "@"); ok && target.User == "" {
		target.User, target.Host = strings.TrimSpace(user), strings.TrimSpace(host)
	}
	if target.Port == 0 {
		target.Port = 22
	}
	if target.Host == "" || target.User == "" || target.Password == "" || target.Port < 1 || target.Port > 65535 {
		return ProbeRequest{}, codedError("invalid_target", nil)
	}
	return target, nil
}
