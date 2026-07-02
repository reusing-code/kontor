package email

import (
	"bufio"
	"fmt"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/reusing-code/kontor/backend/internal/config"
)

// mockSMTPServer starts a simple TCP listener that mocks basic SMTP responses
func mockSMTPServer(t *testing.T) (string, chan string) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to start mock server: %v", err)
	}

	msgs := make(chan string, 1)

	go func() {
		defer l.Close()
		conn, err := l.Accept()
		if err != nil {
			return
		}
		defer conn.Close()

		reader := bufio.NewReader(conn)
		writer := bufio.NewWriter(conn)

		// Initial greeting
		writer.WriteString("220 localhost ESMTP Mock\r\n")
		writer.Flush()

		var dataBuilder strings.Builder
		inData := false

		for {
			line, err := reader.ReadString('\n')
			if err != nil {
				break
			}

			// Capture DATA content
			if inData {
				if line == ".\r\n" {
					inData = false
					msgs <- dataBuilder.String()
					writer.WriteString("250 OK\r\n")
					writer.Flush()
					continue
				}
				dataBuilder.WriteString(line)
				continue
			}

			// Handle commands
			cmd := strings.ToUpper(strings.TrimSpace(line))
			if strings.HasPrefix(cmd, "EHLO") || strings.HasPrefix(cmd, "HELO") {
				writer.WriteString("250-Hello\r\n250 AUTH PLAIN\r\n")
				writer.Flush()
			} else if strings.HasPrefix(cmd, "AUTH PLAIN") {
				writer.WriteString("235 Authentication successful\r\n")
				writer.Flush()
			} else if strings.HasPrefix(cmd, "MAIL FROM") {
				writer.WriteString("250 OK\r\n")
				writer.Flush()
			} else if strings.HasPrefix(cmd, "RCPT TO") {
				writer.WriteString("250 OK\r\n")
				writer.Flush()
			} else if strings.HasPrefix(cmd, "DATA") {
				inData = true
				writer.WriteString("354 Start mail input; end with <CRLF>.<CRLF>\r\n")
				writer.Flush()
			} else if strings.HasPrefix(cmd, "QUIT") {
				writer.WriteString("221 Bye\r\n")
				writer.Flush()
				return
			}
		}
	}()

	return l.Addr().String(), msgs
}

func TestSendEmail(t *testing.T) {
	addr, msgs := mockSMTPServer(t)
	host, portStr, _ := net.SplitHostPort(addr)
	
	// Convert port to int
	var port int
	fmt.Sscanf(portStr, "%d", &port)

	cfg := config.Config{
		SMTPHost:     host,
		SMTPPort:     port,
		SMTPProtocol: "none",
		SMTPUser:     "user",
		SMTPPassword: "password",
		SMTPFrom:     "sender@example.com",
	}

	client := NewClient(cfg)

	to := []string{"recipient@example.com"}
	subject := "Test Subject"
	body := "This is a test email body."

	err := client.Send(to, subject, body)
	if err != nil {
		t.Fatalf("Send() failed: %v", err)
	}

	select {
	case msg := <-msgs:
		if !strings.Contains(msg, "Subject: Test Subject") {
			t.Errorf("expected subject in message, got: %s", msg)
		}
		if !strings.Contains(msg, "This is a test email body.") {
			t.Errorf("expected body in message, got: %s", msg)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for email message")
	}
}
