package codexremote

import (
	"errors"
	"io"
	"net"
	"sync"
	"time"
)

type tunnel struct {
	connection remoteConnection
	listener   net.Listener
	localAddr  string
	done       chan struct{}
	once       sync.Once
	mu         sync.RWMutex
	err        error
}

func startTunnel(connection remoteConnection, remoteAddr, localAddr string) (*tunnel, error) {
	listener, err := connection.Listen("tcp", remoteAddr)
	if err != nil {
		return nil, codedError("tunnel_failed", err)
	}
	value := &tunnel{connection: connection, listener: listener, localAddr: localAddr, done: make(chan struct{})}
	go value.acceptLoop()
	go value.keepAlive()
	return value, nil
}

func (t *tunnel) acceptLoop() {
	for {
		remote, err := t.listener.Accept()
		if err != nil {
			select {
			case <-t.done:
				return
			default:
				t.fail(err)
				return
			}
		}
		go t.forward(remote)
	}
}

func (t *tunnel) forward(remote net.Conn) {
	local, err := net.DialTimeout("tcp", t.localAddr, 5*time.Second)
	if err != nil {
		_ = remote.Close()
		return
	}
	defer remote.Close()
	defer local.Close()
	finished := make(chan struct{}, 2)
	go func() { _, _ = io.Copy(local, remote); finished <- struct{}{} }()
	go func() { _, _ = io.Copy(remote, local); finished <- struct{}{} }()
	<-finished
}

func (t *tunnel) keepAlive() {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-t.done:
			return
		case <-ticker.C:
			if _, _, err := t.connection.SendRequest("keepalive@openssh.com", true, nil); err != nil {
				t.fail(err)
				return
			}
		}
	}
}

func (t *tunnel) fail(err error) {
	t.mu.Lock()
	if t.err == nil {
		t.err = err
	}
	t.mu.Unlock()
	t.Close()
}

func (t *tunnel) Close() error {
	var result error
	t.once.Do(func() {
		close(t.done)
		result = errors.Join(t.listener.Close(), t.connection.Close())
	})
	return result
}

func (t *tunnel) Done() <-chan struct{} { return t.done }

func (t *tunnel) Err() error {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.err
}
