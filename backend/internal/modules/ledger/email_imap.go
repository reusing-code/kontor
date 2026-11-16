package ledger

import (
	"bufio"
	"crypto/tls"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"net"
	"slices"
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

	// uidValidity and folder describe the selected mailbox.
	uidValidity uint32
	folder      string
	caps        map[string]bool
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
	c.uidValidity, c.folder = 0, ""
	lines, err := c.command("SELECT " + imapQuote(imapMailboxName(name)))
	if err != nil {
		return fmt.Errorf("selecting folder: %w", err)
	}
	c.folder = name
	for _, line := range lines {
		if value, ok := imapResponseCode(line, "UIDVALIDITY"); ok {
			if n, err := strconv.ParseUint(value, 10, 32); err == nil {
				c.uidValidity = uint32(n)
			}
		}
	}
	return nil
}

// UIDValidity is the UIDVALIDITY of the selected folder, 0 when the server
// sent none. A UID is only meaningful together with it.
func (c *imapClient) UIDValidity() uint32 {
	return c.uidValidity
}

// imapResponseCode returns the argument of an untagged `* OK [NAME arg]`.
func imapResponseCode(line, name string) (string, bool) {
	trimmed := strings.TrimSpace(line)
	const prefix = "* OK ["
	if len(trimmed) < len(prefix) || !strings.EqualFold(trimmed[:len(prefix)], prefix) {
		return "", false
	}
	code, _, ok := strings.Cut(trimmed[len(prefix):], "]")
	if !ok {
		return "", false
	}
	key, value, _ := strings.Cut(code, " ")
	if !strings.EqualFold(key, name) {
		return "", false
	}
	return strings.TrimSpace(value), true
}

// Capabilities asks the server once per connection; call it after Login.
func (c *imapClient) Capabilities() (map[string]bool, error) {
	if c.caps != nil {
		return c.caps, nil
	}
	lines, err := c.command("CAPABILITY")
	if err != nil {
		return nil, fmt.Errorf("reading capabilities: %w", err)
	}
	caps := map[string]bool{}
	for _, line := range lines {
		fields := strings.Fields(line)
		if len(fields) < 2 || fields[0] != "*" || !strings.EqualFold(fields[1], "CAPABILITY") {
			continue
		}
		for _, name := range fields[2:] {
			caps[strings.ToUpper(name)] = true
		}
	}
	c.caps = caps
	return caps, nil
}

// errIMAPNotExpunged reports a message that was flagged \Deleted but is
// still in the folder: without UIDPLUS only a plain EXPUNGE could remove it,
// and that would also remove every other message the user flagged.
var errIMAPNotExpunged = errors.New("message is flagged as deleted but was not expunged")

// Move moves a message of the selected folder: UID MOVE (RFC 6851) when the
// server has it, else copy, flag and expunge. The server's NO/BAD is an
// errIMAPCommandFailed and leaves the message where it was.
func (c *imapClient) Move(uid uint32, folder string) error {
	caps, err := c.Capabilities()
	if err != nil {
		return err
	}
	target := imapQuote(imapMailboxName(folder))
	if caps["MOVE"] {
		if _, err := c.command(fmt.Sprintf("UID MOVE %d %s", uid, target)); err != nil {
			return fmt.Errorf("moving message: %w", err)
		}
		return nil
	}
	if _, err := c.command(fmt.Sprintf("UID COPY %d %s", uid, target)); err != nil {
		return fmt.Errorf("copying message: %w", err)
	}
	return c.Delete(uid)
}

// Delete flags a message of the selected folder as \Deleted and expunges
// exactly that message. Without UIDPLUS it stays flagged: errIMAPNotExpunged.
func (c *imapClient) Delete(uid uint32) error {
	caps, err := c.Capabilities()
	if err != nil {
		return err
	}
	if _, err := c.command(fmt.Sprintf(`UID STORE %d +FLAGS.SILENT (\Deleted)`, uid)); err != nil {
		return fmt.Errorf("flagging message: %w", err)
	}
	if !caps["UIDPLUS"] {
		return errIMAPNotExpunged
	}
	if _, err := c.command(fmt.Sprintf("UID EXPUNGE %d", uid)); err != nil {
		return fmt.Errorf("expunging message: %w", err)
	}
	return nil
}

// SearchHeader returns the UIDs of the selected folder whose header field
// contains the value. The value must not contain control characters.
func (c *imapClient) SearchHeader(field, value string) ([]uint32, error) {
	lines, err := c.command(fmt.Sprintf("UID SEARCH HEADER %s %s", field, imapQuote(value)))
	if err != nil {
		return nil, fmt.Errorf("searching mailbox: %w", err)
	}
	return parseIMAPSearch(lines), nil
}

func parseIMAPSearch(lines []string) []uint32 {
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
		return result
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
	return parseIMAPSearch(lines), nil
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
			if !imapTaggedOK(trimmed, tag) {
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
			if !imapTaggedOK(trimmed, tag) {
				return lines, fmt.Errorf("%w: %s", errIMAPCommandFailed, trimmed)
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

// imapTaggedOK reports whether the tagged response line is an OK. The text
// after the status is the server's and may itself contain "OK".
func imapTaggedOK(line, tag string) bool {
	status, _, _ := strings.Cut(strings.TrimPrefix(line, tag+" "), " ")
	return strings.EqualFold(status, "OK")
}

// errIMAPCommandFailed marks a tagged NO/BAD answer to a plain command, as
// opposed to an I/O failure.
var errIMAPCommandFailed = errors.New("imap command failed")

const (
	maxIMAPFolders         = 2000
	maxIMAPFolderNameBytes = 1024
)

// imapMailbox is one selectable mailbox with its lower-cased LIST flags.
type imapMailbox struct {
	Name  string
	Flags []string
}

// ListFolders returns the selectable mailboxes with their names decoded from
// modified UTF-7. The names contain the server's hierarchy delimiter as is.
func (c *imapClient) ListFolders() ([]string, error) {
	boxes, err := c.listMailboxes()
	if err != nil {
		return nil, err
	}
	names := make([]string, 0, len(boxes))
	for _, box := range boxes {
		names = append(names, box.Name)
	}
	return names, nil
}

// TrashFolder returns the mailbox the server marks as \Trash (RFC 6154), or
// "" when it marks none.
func (c *imapClient) TrashFolder() (string, error) {
	boxes, err := c.listMailboxes()
	if err != nil {
		return "", err
	}
	for _, box := range boxes {
		if slices.Contains(box.Flags, `\trash`) {
			return box.Name, nil
		}
	}
	return "", nil
}

func (c *imapClient) listMailboxes() ([]imapMailbox, error) {
	tag := c.nextTag()
	if err := c.writeLine(tag + ` LIST "" "*"`); err != nil {
		return nil, err
	}
	folders := make([]imapMailbox, 0)
	for {
		line, err := c.readLine()
		if err != nil {
			return nil, err
		}
		trimmed := strings.TrimRight(line, "\r\n")
		if strings.HasPrefix(trimmed, tag+" ") {
			if !imapTaggedOK(trimmed, tag) {
				return nil, fmt.Errorf("%w: %s", errIMAPCommandFailed, trimmed)
			}
			return folders, nil
		}
		// a server may send the mailbox name as a literal
		if size, ok := parseIMAPLiteralSize(line); ok {
			c.extendDeadline()
			if size > maxIMAPFolderNameBytes {
				if _, err := io.CopyN(io.Discard, c.r, int64(size)); err != nil {
					return nil, err
				}
				continue
			}
			name := make([]byte, size)
			if _, err := io.ReadFull(c.r, name); err != nil {
				return nil, err
			}
			if _, err := c.readLine(); err != nil {
				return nil, err
			}
			trimmed = trimmed[:strings.LastIndex(trimmed, "{")] + imapQuote(string(name))
		}
		if box, ok := parseIMAPListEntry(trimmed); ok && len(folders) < maxIMAPFolders {
			folders = append(folders, box)
		}
	}
}

// parseIMAPListLine reads one untagged LIST response,
// `* LIST (flags) delimiter mailbox`, and returns the decoded mailbox name.
// Mailboxes that cannot be selected and names the account could not store
// are left out.
func parseIMAPListLine(line string) (string, bool) {
	box, ok := parseIMAPListEntry(line)
	return box.Name, ok
}

func parseIMAPListEntry(line string) (imapMailbox, bool) {
	const prefix = "* LIST "
	if len(line) < len(prefix) || !strings.EqualFold(line[:len(prefix)], prefix) {
		return imapMailbox{}, false
	}
	rest := strings.TrimLeft(line[len(prefix):], " ")
	if !strings.HasPrefix(rest, "(") {
		return imapMailbox{}, false
	}
	end := strings.Index(rest, ")")
	if end == -1 {
		return imapMailbox{}, false
	}
	flags := strings.Fields(strings.ToLower(rest[1:end]))
	if slices.Contains(flags, `\noselect`) || slices.Contains(flags, `\nonexistent`) {
		return imapMailbox{}, false
	}
	_, rest, ok := parseIMAPAString(rest[end+1:])
	if !ok {
		return imapMailbox{}, false
	}
	raw, rest, ok := parseIMAPAString(rest)
	if !ok || strings.TrimSpace(rest) != "" {
		return imapMailbox{}, false
	}
	name, err := normalizeEmailFolder(decodeIMAPMailboxName(raw))
	if err != nil {
		return imapMailbox{}, false
	}
	return imapMailbox{Name: name, Flags: flags}, true
}

// parseIMAPAString reads a quoted string or an atom (which covers NIL) and
// returns it with the remainder of the line.
func parseIMAPAString(s string) (value string, rest string, ok bool) {
	s = strings.TrimLeft(s, " ")
	if s == "" {
		return "", "", false
	}
	if s[0] != '"' {
		value, rest, _ = strings.Cut(s, " ")
		return value, rest, true
	}
	var out strings.Builder
	for i := 1; i < len(s); i++ {
		switch s[i] {
		case '\\':
			i++
			if i == len(s) {
				return "", "", false
			}
			out.WriteByte(s[i])
		case '"':
			return out.String(), s[i+1:], true
		default:
			out.WriteByte(s[i])
		}
	}
	return "", "", false
}

// decodeIMAPMailboxName is the inverse of imapMailboxName. A name that is
// not valid modified UTF-7 is returned unchanged.
func decodeIMAPMailboxName(name string) string {
	var out strings.Builder
	for i := 0; i < len(name); i++ {
		if name[i] != '&' {
			out.WriteByte(name[i])
			continue
		}
		end := strings.IndexByte(name[i:], '-')
		if end == -1 {
			return name
		}
		encoded := name[i+1 : i+end]
		i += end
		if encoded == "" {
			out.WriteByte('&')
			continue
		}
		buf, err := base64.RawStdEncoding.DecodeString(strings.ReplaceAll(encoded, ",", "/"))
		if err != nil || len(buf)%2 != 0 {
			return name
		}
		units := make([]uint16, 0, len(buf)/2)
		for j := 0; j < len(buf); j += 2 {
			units = append(units, uint16(buf[j])<<8|uint16(buf[j+1]))
		}
		out.WriteString(string(utf16.Decode(units)))
	}
	return out.String()
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
