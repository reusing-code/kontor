package email

import (
	"bufio"
	"errors"
	"fmt"
	"mime"
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

func mockClient(t *testing.T) (*Client, chan string) {
	t.Helper()
	addr, msgs := mockSMTPServer(t)
	host, portStr, _ := net.SplitHostPort(addr)
	var port int
	fmt.Sscanf(portStr, "%d", &port)
	return NewClient(config.Config{SMTPHost: host, SMTPPort: port, SMTPProtocol: "none", SMTPFrom: "sender@example.com"}), msgs
}

// closedPortClient points at a port nothing listens on: a Send that reaches
// the network fails with a connection error instead of a validation error.
func closedPortClient(t *testing.T) *Client {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := l.Addr().(*net.TCPAddr).Port
	l.Close()
	return NewClient(config.Config{SMTPHost: "127.0.0.1", SMTPPort: port, SMTPProtocol: "none", SMTPFrom: "sender@example.com"})
}

func received(t *testing.T, msgs chan string) string {
	t.Helper()
	select {
	case msg := <-msgs:
		return msg
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for email message")
		return ""
	}
}

func headerLines(msg string) []string {
	head, _, _ := strings.Cut(msg, "\r\n\r\n")
	return strings.Split(head, "\r\n")
}

func TestSendRejectsHeaderInjection(t *testing.T) {
	for name, c := range map[string]struct {
		to      []string
		subject string
		want    error
	}{
		"subject CRLF Bcc":   {[]string{"a@example.com"}, "Hi\r\nBcc: evil@example.com", ErrInvalidSubject},
		"subject bare LF":    {[]string{"a@example.com"}, "Hi\nBcc: evil@example.com", ErrInvalidSubject},
		"subject bare CR":    {[]string{"a@example.com"}, "Hi\rBcc: evil@example.com", ErrInvalidSubject},
		"subject early body": {[]string{"a@example.com"}, "Hi\r\n\r\nbody", ErrInvalidSubject},
		"subject NUL":        {[]string{"a@example.com"}, "Hi\x00", ErrInvalidSubject},
		"recipient CRLF Bcc": {[]string{"a@example.com\r\nBcc: evil@example.com"}, "Hi", ErrInvalidRecipient},
		"recipient folded":   {[]string{"a@example.com\r\n Bcc: evil@example.com"}, "Hi", ErrInvalidRecipient},
		"second recipient":   {[]string{"a@example.com", "b@example.com\nBcc: evil@example.com"}, "Hi", ErrInvalidRecipient},
		"recipient list":     {[]string{"a@example.com, evil@example.com"}, "Hi", ErrInvalidRecipient},
		"not an address":     {[]string{"not an address"}, "Hi", ErrInvalidRecipient},
		"empty address":      {[]string{""}, "Hi", ErrInvalidRecipient},
		"no recipients":      {nil, "Hi", ErrInvalidRecipient},
	} {
		t.Run(name, func(t *testing.T) {
			err := closedPortClient(t).Send(c.to, c.subject, "body")
			if !errors.Is(err, c.want) {
				t.Fatalf("got %v, want %v", err, c.want)
			}
			if strings.Contains(err.Error(), "example.com") {
				t.Fatalf("error carries an address: %v", err)
			}
		})
	}
}

func TestSendRejectsControlCharactersInSender(t *testing.T) {
	client := closedPortClient(t)
	client.from = "sender@example.com\r\nBcc: evil@example.com"
	if err := client.Send([]string{"a@example.com"}, "Hi", "body"); !errors.Is(err, ErrInvalidSender) {
		t.Fatalf("got %v", err)
	}
}

func TestSendWritesOnlyTheExpectedHeaders(t *testing.T) {
	client, msgs := mockClient(t)
	if err := client.Send([]string{"Jane Doe <jane@example.com>", "b@example.com"}, "Plain subject", "line one\r\nBcc: in the body is harmless"); err != nil {
		t.Fatalf("Send() failed: %v", err)
	}
	want := []string{
		"From: sender@example.com",
		"To: jane@example.com, b@example.com",
		"Subject: Plain subject",
		"MIME-Version: 1.0",
		`Content-Type: text/plain; charset="utf-8"`,
	}
	if got := headerLines(received(t, msgs)); strings.Join(got, "|") != strings.Join(want, "|") {
		t.Fatalf("headers = %q", got)
	}
}

func TestSendEncodesNonASCIISubject(t *testing.T) {
	for _, subject := range []string{
		"Kontor: Verbindung zu Sparkasse Köln läuft ab",
		strings.Repeat("Überweisungsbestätigung für Ihre Bankverbindung ", 6),
	} {
		client, msgs := mockClient(t)
		if err := client.Send([]string{"a@example.com"}, subject, "body"); err != nil {
			t.Fatalf("Send() failed: %v", err)
		}
		var header string
		for _, line := range headerLines(received(t, msgs)) {
			if v, ok := strings.CutPrefix(line, "Subject: "); ok {
				header = v
			}
		}
		if header == "" || strings.ContainsFunc(header, func(r rune) bool { return r > 126 || r < 32 }) {
			t.Fatalf("subject header is not printable ASCII: %q", header)
		}
		decoded, err := new(mime.WordDecoder).DecodeHeader(header)
		if err != nil || decoded != subject {
			t.Fatalf("decoded %q (%v), want %q", decoded, err, subject)
		}
	}
}

func TestSendRcptErrorCarriesNoAddress(t *testing.T) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	go func() {
		conn, err := l.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		r := bufio.NewReader(conn)
		fmt.Fprint(conn, "220 mock\r\n")
		for {
			line, err := r.ReadString('\n')
			if err != nil {
				return
			}
			switch cmd := strings.ToUpper(line); {
			case strings.HasPrefix(cmd, "RCPT TO"):
				fmt.Fprint(conn, "550 no such user\r\n")
			case strings.HasPrefix(cmd, "QUIT"):
				fmt.Fprint(conn, "221 Bye\r\n")
				return
			default:
				fmt.Fprint(conn, "250 OK\r\n")
			}
		}
	}()
	port := l.Addr().(*net.TCPAddr).Port
	client := NewClient(config.Config{SMTPHost: "127.0.0.1", SMTPPort: port, SMTPProtocol: "none", SMTPFrom: "sender@example.com"})
	err = client.Send([]string{"private@example.com"}, "Hi", "body")
	if err == nil || strings.Contains(err.Error(), "private@example.com") {
		t.Fatalf("got %v", err)
	}
}
