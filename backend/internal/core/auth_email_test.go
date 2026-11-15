package core

import (
	"bytes"
	"log/slog"
	"net"
	"strings"
	"testing"

	"github.com/reusing-code/kontor/backend/internal/config"
	"github.com/reusing-code/kontor/backend/internal/email"
	"github.com/reusing-code/kontor/backend/internal/module"
)

func TestWelcomeEmailFailureLogsNoAddress(t *testing.T) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := l.Addr().(*net.TCPAddr).Port
	l.Close()
	client := email.NewClient(config.Config{SMTPHost: "127.0.0.1", SMTPPort: port, SMTPProtocol: "none", SMTPFrom: "kontor@example.com"})

	for name, to := range map[string]string{
		"server unreachable": "private.person@example.org",
		"refused address":    "private.person@example.org\r\nBcc: other@example.org",
	} {
		t.Run(name, func(t *testing.T) {
			logs := &bytes.Buffer{}
			h := NewHandler(newTestStore(t), slog.New(slog.NewTextHandler(logs, nil)), testJWTSecret, client, nil, module.NewRegistry())
			h.sendWelcomeEmail(to)
			if !strings.Contains(logs.String(), "sending welcome email") {
				t.Fatalf("failure not logged: %s", logs)
			}
			if strings.Contains(logs.String(), "example.org") || strings.Contains(logs.String(), "private.person") {
				t.Fatalf("log carries the address: %s", logs)
			}
		})
	}
}
