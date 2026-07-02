package email

import (
	"crypto/tls"
	"fmt"
	"net"
	"net/smtp"
	"strings"
	"time"

	"github.com/reusing-code/kontor/backend/internal/config"
)

type Client struct {
	host     string
	port     int
	protocol string
	user     string
	password string
	from     string
}

func NewClient(cfg config.Config) *Client {
	return &Client{
		host:     cfg.SMTPHost,
		port:     cfg.SMTPPort,
		protocol: cfg.SMTPProtocol,
		user:     cfg.SMTPUser,
		password: cfg.SMTPPassword,
		from:     cfg.SMTPFrom,
	}
}

func (c *Client) IsConfigured() bool { return c.host != "" }

func (c *Client) Send(to []string, subject, body string) error {
	if c.host == "" {
		return fmt.Errorf("SMTP host not configured")
	}

	addr := fmt.Sprintf("%s:%d", c.host, c.port)

	var conn net.Conn
	var err error

	tlsConfig := &tls.Config{
		InsecureSkipVerify: false,
		ServerName:         c.host,
	}

	if c.protocol == "tls" {
		conn, err = tls.Dial("tcp", addr, tlsConfig)
	} else {
		conn, err = net.DialTimeout("tcp", addr, 10*time.Second)
	}

	if err != nil {
		return fmt.Errorf("failed to connect to SMTP server: %w", err)
	}
	defer conn.Close()

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

	for _, addr := range to {
		if err = client.Rcpt(addr); err != nil {
			return fmt.Errorf("failed to set RCPT TO for %s: %w", addr, err)
		}
	}

	w, err := client.Data()
	if err != nil {
		return fmt.Errorf("failed to open DATA writer: %w", err)
	}

	// Simple email headers
	headers := make(map[string]string)
	headers["From"] = c.from
	headers["To"] = strings.Join(to, ",")
	headers["Subject"] = subject
	headers["MIME-Version"] = "1.0"
	headers["Content-Type"] = "text/plain; charset=\"utf-8\""

	message := ""
	for k, v := range headers {
		message += fmt.Sprintf("%s: %s\r\n", k, v)
	}
	message += "\r\n" + body

	if _, err = w.Write([]byte(message)); err != nil {
		return fmt.Errorf("failed to write message body: %w", err)
	}

	if err = w.Close(); err != nil {
		return fmt.Errorf("failed to close DATA writer: %w", err)
	}

	return nil
}
