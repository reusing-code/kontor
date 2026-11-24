package email

import (
	"net"
	"testing"
	"time"

	"github.com/reusing-code/kontor/backend/internal/config"
)

// stalledServer accepts connections and never says anything.
func stalledServer(t *testing.T) (string, int) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	conns := make(chan net.Conn, 4)
	t.Cleanup(func() {
		ln.Close()
		close(conns)
		for c := range conns {
			c.Close()
		}
	})
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			conns <- c
		}
	}()
	addr := ln.Addr().(*net.TCPAddr)
	return addr.IP.String(), addr.Port
}

func TestSend_DoesNotHangOnAStalledServer(t *testing.T) {
	for _, protocol := range []string{"none", "starttls", "tls"} {
		t.Run(protocol, func(t *testing.T) {
			host, port := stalledServer(t)
			c := NewClient(config.Config{SMTPHost: host, SMTPPort: port, SMTPProtocol: protocol, SMTPFrom: "sender@example.com"})
			c.dialTimeout, c.sendTimeout = 200*time.Millisecond, 200*time.Millisecond

			done := make(chan error, 1)
			go func() { done <- c.Send([]string{"to@example.com"}, "s", "b") }()
			select {
			case err := <-done:
				if err == nil {
					t.Fatal("Send succeeded against a silent server")
				}
			case <-time.After(5 * time.Second):
				t.Fatal("Send still blocked after 5s")
			}
		})
	}
}

func TestNewClient_SetsTimeouts(t *testing.T) {
	c := NewClient(config.Config{})
	if c.dialTimeout != defaultDialTimeout || c.sendTimeout != defaultSendTimeout {
		t.Errorf("timeouts = %v, %v", c.dialTimeout, c.sendTimeout)
	}
}
