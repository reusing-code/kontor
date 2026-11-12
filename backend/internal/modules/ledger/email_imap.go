package ledger

import (
	"bufio"
	"crypto/tls"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"net"
	"strconv"
	"strings"
	"time"
	"unicode/utf16"
)

const (
	imapDialTimeout = 15 * time.Second
	imapIOTimeout   = 60 * time.Second
)

type imapClient struct {
	conn   net.Conn
	r      *bufio.Reader
	w      *bufio.Writer
	tagNum int
}

func newIMAPClient(host string, port int, useTLS bool) (*imapClient, error) {
	address := net.JoinHostPort(host, strconv.Itoa(port))
	dialer := &net.Dialer{Timeout: imapDialTimeout}
	var (
		conn net.Conn
		err  error
	)
	if useTLS {
		conn, err = tls.DialWithDialer(dialer, "tcp", address, &tls.Config{ServerName: host, MinVersion: tls.VersionTLS12})
	} else {
		conn, err = dialer.Dial("tcp", address)
	}
	if err != nil {
		return nil, fmt.Errorf("connecting to imap: %w", err)
	}
	client := &imapClient{conn: conn, r: bufio.NewReader(conn), w: bufio.NewWriter(conn)}
	line, err := client.readLine()
	if err != nil {
		conn.Close()
		return nil, fmt.Errorf("reading imap greeting: %w", err)
	}
	if !strings.HasPrefix(line, "*") {
		conn.Close()
		return nil, fmt.Errorf("unexpected imap greeting: %s", strings.TrimSpace(line))
	}
	return client, nil
}

func (c *imapClient) Close() error {
	if c.conn == nil {
		return nil
	}
	return c.conn.Close()
}

func (c *imapClient) Login(username string, password string) error {
	_, err := c.command(fmt.Sprintf("LOGIN %s %s", imapQuote(username), imapQuote(password)))
	if err != nil {
		return fmt.Errorf("logging in to imap: %w", err)
	}
	return nil
}

func (c *imapClient) Logout() {
	_, _ = c.command("LOGOUT")
}

func (c *imapClient) SelectFolder(name string) error {
	_, err := c.command("SELECT " + imapQuote(imapMailboxName(name)))
	if err != nil {
		return fmt.Errorf("selecting folder: %w", err)
	}
	return nil
}

func (c *imapClient) SearchSince(scanSince string) ([]uint32, error) {
	date, err := time.Parse("2006-01-02", scanSince)
	if err != nil {
		return nil, fmt.Errorf("invalid scan start date: %w", err)
	}
	lines, err := c.command(fmt.Sprintf("UID SEARCH SINCE %s", date.Format("2-Jan-2006")))
	if err != nil {
		return nil, fmt.Errorf("searching mailbox: %w", err)
	}
	for _, line := range lines {
		if !strings.HasPrefix(line, "* SEARCH") {
			continue
		}
		fields := strings.Fields(strings.TrimSpace(strings.TrimPrefix(line, "* SEARCH")))
		result := make([]uint32, 0, len(fields))
		for _, field := range fields {
			value, err := strconv.ParseUint(field, 10, 32)
			if err != nil {
				continue
			}
			result = append(result, uint32(value))
		}
		return result, nil
	}
	return nil, nil
}

// FetchMessage downloads the whole message.
func (c *imapClient) FetchMessage(uid uint32) ([]byte, error) {
	return c.fetchLiteral(uid, "")
}

// FetchHeader downloads only the header block of the message.
func (c *imapClient) FetchHeader(uid uint32) ([]byte, error) {
	return c.fetchLiteral(uid, "HEADER")
}

// maxIMAPLiteralBytes bounds what is read into memory for one message.
const maxIMAPLiteralBytes = 25 << 20

var (
	// errIMAPRejected marks a command the server answered with NO/BAD: the
	// connection is still usable. Other fetch errors are I/O failures.
	errIMAPRejected    = errors.New("imap command rejected")
	errMessageTooLarge = errors.New("message is too large")
)

func (c *imapClient) fetchLiteral(uid uint32, section string) ([]byte, error) {
	tag := c.nextTag()
	if err := c.writeLine(fmt.Sprintf("%s UID FETCH %d (BODY.PEEK[%s])", tag, uid, section)); err != nil {
		return nil, err
	}
	var body []byte
	tooLarge := false
	for {
		line, err := c.readLine()
		if err != nil {
			return nil, err
		}
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, tag+" ") {
			if !strings.Contains(trimmed, "OK") {
				return nil, fmt.Errorf("%w: %s", errIMAPRejected, trimmed)
			}
			break
		}
		literalSize, ok := parseIMAPLiteralSize(line)
		if ok && literalSize > maxIMAPLiteralBytes {
			c.extendDeadline()
			if _, err := io.CopyN(io.Discard, c.r, int64(literalSize)); err != nil {
				return nil, fmt.Errorf("skipping message literal: %w", err)
			}
			tooLarge = true
			continue
		}
		if ok {
			body = make([]byte, literalSize)
			c.extendDeadline()
			if _, err := io.ReadFull(c.r, body); err != nil {
				return nil, fmt.Errorf("reading message literal: %w", err)
			}
		}
	}
	if tooLarge {
		return nil, errMessageTooLarge
	}
	if len(body) == 0 {
		return nil, fmt.Errorf("%w: empty message data for uid %d", errIMAPRejected, uid)
	}
	return body, nil
}

func (c *imapClient) command(command string) ([]string, error) {
	tag := c.nextTag()
	if err := c.writeLine(fmt.Sprintf("%s %s", tag, command)); err != nil {
		return nil, err
	}
	lines := make([]string, 0)
	for {
		line, err := c.readLine()
		if err != nil {
			return nil, err
		}
		lines = append(lines, line)
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, tag+" ") {
			if !strings.Contains(trimmed, "OK") {
				return lines, fmt.Errorf("imap command failed: %s", trimmed)
			}
			return lines, nil
		}
		if literalSize, ok := parseIMAPLiteralSize(line); ok {
			c.extendDeadline()
			if _, err := io.CopyN(io.Discard, c.r, int64(literalSize)); err != nil {
				return nil, err
			}
		}
	}
}

func (c *imapClient) nextTag() string {
	c.tagNum++
	return fmt.Sprintf("A%04d", c.tagNum)
}

// extendDeadline bounds the next read/write so a stalled IMAP server
// cannot block the calling request indefinitely.
func (c *imapClient) extendDeadline() {
	_ = c.conn.SetDeadline(time.Now().Add(imapIOTimeout))
}

func (c *imapClient) writeLine(line string) error {
	c.extendDeadline()
	if _, err := c.w.WriteString(line + "\r\n"); err != nil {
		return err
	}
	return c.w.Flush()
}

func (c *imapClient) readLine() (string, error) {
	c.extendDeadline()
	return c.r.ReadString('\n')
}

func imapQuote(value string) string {
	escaped := strings.ReplaceAll(value, `\`, `\\`)
	escaped = strings.ReplaceAll(escaped, `"`, `\"`)
	return `"` + escaped + `"`
}

// imapMailboxName encodes a folder name as IMAP modified UTF-7 (RFC 3501, 5.1.3).
func imapMailboxName(name string) string {
	var out strings.Builder
	var run []uint16
	flush := func() {
		if len(run) == 0 {
			return
		}
		buf := make([]byte, 0, len(run)*2)
		for _, unit := range run {
			buf = append(buf, byte(unit>>8), byte(unit))
		}
		out.WriteByte('&')
		out.WriteString(strings.ReplaceAll(base64.RawStdEncoding.EncodeToString(buf), "/", ","))
		out.WriteByte('-')
		run = run[:0]
	}
	for _, r := range name {
		switch {
		case r == '&':
			flush()
			out.WriteString("&-")
		case r >= 0x20 && r <= 0x7e:
			flush()
			out.WriteRune(r)
		default:
			run = append(run, utf16.Encode([]rune{r})...)
		}
	}
	flush()
	return out.String()
}

func parseIMAPLiteralSize(line string) (int, bool) {
	trimmed := strings.TrimRight(line, "\r\n")
	start := strings.LastIndex(trimmed, "{")
	end := strings.LastIndex(trimmed, "}")
	if start == -1 || end == -1 || end <= start+1 || end != len(trimmed)-1 {
		return 0, false
	}
	size, err := strconv.Atoi(trimmed[start+1 : end])
	if err != nil || size < 0 {
		return 0, false
	}
	return size, true
}
