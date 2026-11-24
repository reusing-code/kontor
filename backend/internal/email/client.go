package email

import (
	"crypto/tls"
	"errors"
	"fmt"
	"mime"
	"net"
	"net/mail"
	"net/smtp"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/reusing-code/kontor/backend/internal/config"
)

// A stalled server must not block the reminder loop: the dial has its own
// limit and the whole conversation one deadline.
const (
	defaultDialTimeout = 10 * time.Second
	defaultSendTimeout = 60 * time.Second
)

type Client struct {
	host     string
	port     int
	protocol string
	user     string
	password string
	from     string

	dialTimeout time.Duration
	sendTimeout time.Duration
}

func NewClient(cfg config.Config) *Client {
	return &Client{
		host:        cfg.SMTPHost,
		port:        cfg.SMTPPort,
		protocol:    cfg.SMTPProtocol,
		user:        cfg.SMTPUser,
		password:    cfg.SMTPPassword,
		from:        cfg.SMTPFrom,
		dialTimeout: defaultDialTimeout,
		sendTimeout: defaultSendTimeout,
	}
}

func (c *Client) IsConfigured() bool { return c.host != "" }

var (
	ErrInvalidRecipient = errors.New("invalid recipient address")
	ErrInvalidSubject   = errors.New("invalid subject")
	ErrInvalidSender    = errors.New("invalid sender address")
)

// Send refuses header values with control characters instead of writing them:
// a CR or LF would end the header and let the value add headers of its own.
// Errors never carry an address.
func (c *Client) Send(to []string, subject, body string) error {
	if c.host == "" {
		return fmt.Errorf("SMTP host not configured")
	}

	recipients, err := parseRecipients(to)
	if err != nil {
		return err
	}
	message, err := buildMessage(c.from, recipients, subject, body)
	if err != nil {
		return err
	}

	addr := net.JoinHostPort(c.host, strconv.Itoa(c.port))

	var conn net.Conn

	tlsConfig := &tls.Config{
		InsecureSkipVerify: false,
		ServerName:         c.host,
	}

	dialer := &net.Dialer{Timeout: c.dialTimeout}
	if c.protocol == "tls" {
		conn, err = tls.DialWithDialer(dialer, "tcp", addr, tlsConfig)
	} else {
		conn, err = dialer.Dial("tcp", addr)
	}

	if err != nil {
		return fmt.Errorf("failed to connect to SMTP server: %w", err)
	}
	defer conn.Close()
	if err := conn.SetDeadline(time.Now().Add(c.sendTimeout)); err != nil {
		return fmt.Errorf("setting SMTP deadline: %w", err)
	}

	client, err := smtp.NewClient(conn, c.host)
	if err != nil {
		return fmt.Errorf("failed to create SMTP client: %w", err)
	}
	defer client.Quit()

	if c.protocol == "starttls" {
		if err = client.StartTLS(tlsConfig); err != nil {
			return fmt.Errorf("failed to start TLS: %w", err)
		}
	}

	if c.user != "" && c.password != "" {
		auth := smtp.PlainAuth("", c.user, c.password, c.host)
		if err = client.Auth(auth); err != nil {
			return fmt.Errorf("failed to authenticate: %w", err)
		}
	}

	if err = client.Mail(c.from); err != nil {
		return fmt.Errorf("failed to set MAIL FROM: %w", err)
	}

	for _, rcpt := range recipients {
		if err = client.Rcpt(rcpt); err != nil {
			return fmt.Errorf("failed to set RCPT TO: %w", err)
		}
	}

	w, err := client.Data()
	if err != nil {
		return fmt.Errorf("failed to open DATA writer: %w", err)
	}

	if _, err = w.Write(message); err != nil {
		return fmt.Errorf("failed to write message body: %w", err)
	}

	if err = w.Close(); err != nil {
		return fmt.Errorf("failed to close DATA writer: %w", err)
	}

	return nil
}

func hasControl(s string) bool {
	return strings.ContainsFunc(s, unicode.IsControl)
}

// parseRecipients returns the bare addresses, which is what both RCPT TO and
// the To header get.
func parseRecipients(to []string) ([]string, error) {
	if len(to) == 0 {
		return nil, ErrInvalidRecipient
	}
	recipients := make([]string, 0, len(to))
	for _, raw := range to {
		if hasControl(raw) {
			return nil, ErrInvalidRecipient
		}
		// the parser's error can quote its input, so it is not wrapped
		addr, err := mail.ParseAddress(raw)
		if err != nil || hasControl(addr.Address) {
			return nil, ErrInvalidRecipient
		}
		recipients = append(recipients, addr.Address)
	}
	return recipients, nil
}

func buildMessage(from string, recipients []string, subject, body string) ([]byte, error) {
	if hasControl(from) {
		return nil, ErrInvalidSender
	}
	if hasControl(subject) {
		return nil, ErrInvalidSubject
	}
	var b strings.Builder
	for _, h := range [][2]string{
		{"From", from},
		{"To", strings.Join(recipients, ", ")},
		{"Subject", mime.QEncoding.Encode("utf-8", subject)},
		{"MIME-Version", "1.0"},
		{"Content-Type", `text/plain; charset="utf-8"`},
	} {
		b.WriteString(h[0] + ": " + h[1] + "\r\n")
	}
	b.WriteString("\r\n" + body)
	return []byte(b.String()), nil
}
