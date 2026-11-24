package ledger

import (
	"bufio"
	"bytes"
	"encoding/base64"
	"errors"
	"fmt"
	"html"
	"io"
	"mime"
	"mime/multipart"
	"mime/quotedprintable"
	"net/mail"
	"net/textproto"
	"regexp"
	"strings"
	"time"

	"golang.org/x/net/html/charset"
)

type ParsedEmail struct {
	MessageID string
	From      string
	Subject   string
	Date      time.Time
	TextBody  string
	HTMLBody  string
}

// headerFields extracts the identifying header fields of a message.
func headerFields(h mail.Header) ParsedEmail {
	subject := strings.TrimSpace(h.Get("Subject"))
	if decodedSubject, err := decodeHeader(subject); err == nil && decodedSubject != "" {
		subject = decodedSubject
	}
	from := strings.TrimSpace(h.Get("From"))
	if decodedFrom, err := decodeHeader(from); err == nil && decodedFrom != "" {
		from = decodedFrom
	}
	parsed := ParsedEmail{
		MessageID: strings.TrimSpace(h.Get("Message-Id")),
		From:      from,
		Subject:   subject,
	}
	if dateHeader := strings.TrimSpace(h.Get("Date")); dateHeader != "" {
		if parsedDate, err := mail.ParseDate(dateHeader); err == nil {
			parsed.Date = parsedDate.UTC()
		}
	}
	return parsed
}

var errMessageHeaders = errors.New("could not parse message headers")

// parseHeader reads the header fields of a raw header block (or a whole
// message). Its error is fixed text: net/mail errors can quote header lines.
func parseHeader(raw []byte) (ParsedEmail, error) {
	msg, err := mail.ReadMessage(bytes.NewReader(append(headerBlock(raw), '\n', '\n')))
	if err != nil {
		return ParsedEmail{}, errMessageHeaders
	}
	return headerFields(msg.Header), nil
}

// headerBlock returns the bytes before the first blank line, without it.
func headerBlock(raw []byte) []byte {
	for i := 0; i < len(raw); i++ {
		if raw[i] != '\n' {
			continue
		}
		rest := raw[i+1:]
		if len(rest) > 0 && rest[0] == '\n' || len(rest) > 1 && rest[0] == '\r' && rest[1] == '\n' {
			return bytes.TrimRight(raw[:i], "\r")
		}
	}
	return bytes.TrimRight(raw, "\r\n")
}

func parseMessage(r io.Reader) (ParsedEmail, error) {
	msg, err := mail.ReadMessage(bufio.NewReader(r))
	if err != nil {
		return ParsedEmail{}, fmt.Errorf("reading message: %w", err)
	}
	parsed := headerFields(msg.Header)
	if err := collectBodies(&parsed, textproto.MIMEHeader(msg.Header), msg.Body, 0); err != nil {
		return ParsedEmail{}, err
	}
	if parsed.TextBody == "" && parsed.HTMLBody != "" {
		parsed.TextBody = stripHTML(parsed.HTMLBody)
	}
	return parsed, nil
}

// maxMIMEDepth bounds the nesting of multipart parts that is walked.
const maxMIMEDepth = 10

// collectBodies walks a MIME entity depth first and keeps the first
// text/plain and the first text/html body that is not an attachment, decoded
// to UTF-8. An entity without (or with an unparsable) Content-Type is plain
// text.
func collectBodies(parsed *ParsedEmail, header textproto.MIMEHeader, body io.Reader, depth int) error {
	mediaType, params, err := mime.ParseMediaType(header.Get("Content-Type"))
	if err != nil {
		mediaType, params = "text/plain", nil
	}
	if strings.HasPrefix(mediaType, "multipart/") {
		if depth >= maxMIMEDepth {
			return nil
		}
		reader := multipart.NewReader(body, params["boundary"])
		for {
			part, err := reader.NextPart()
			if err == io.EOF {
				return nil
			}
			if err != nil {
				return fmt.Errorf("reading multipart body: %w", err)
			}
			if isAttachment(part.Header) {
				continue
			}
			if err := collectBodies(parsed, part.Header, part, depth+1); err != nil {
				return err
			}
		}
	}

	var target *string
	switch {
	case mediaType == "text/plain" && parsed.TextBody == "":
		target = &parsed.TextBody
	case mediaType == "text/html" && parsed.HTMLBody == "":
		target = &parsed.HTMLBody
	default:
		return nil
	}
	raw, err := io.ReadAll(body)
	if err != nil {
		return fmt.Errorf("reading %s body: %w", mediaType, err)
	}
	decoded, err := decodeBody(raw, header.Get("Content-Transfer-Encoding"))
	if err != nil {
		return fmt.Errorf("decoding %s body: %w", mediaType, err)
	}
	*target = decodeCharset(decoded, params["charset"])
	return nil
}

func isAttachment(header textproto.MIMEHeader) bool {
	disposition, params, err := mime.ParseMediaType(header.Get("Content-Disposition"))
	return err == nil && (disposition == "attachment" || params["filename"] != "")
}

// decodeCharset converts a body in the given charset to UTF-8; an unknown
// charset leaves the bytes as they are.
func decodeCharset(body []byte, label string) string {
	label = strings.ToLower(strings.TrimSpace(label))
	if label == "" || label == "utf-8" || label == "us-ascii" {
		return string(body)
	}
	reader, err := charset.NewReaderLabel(label, bytes.NewReader(body))
	if err != nil {
		return string(body)
	}
	decoded, err := io.ReadAll(reader)
	if err != nil {
		return string(body)
	}
	return string(decoded)
}

// decodeBody undoes the transfer encoding. Base64 is read leniently: all
// whitespace is dropped and missing padding is accepted.
func decodeBody(body []byte, transferEncoding string) ([]byte, error) {
	switch strings.ToLower(strings.TrimSpace(transferEncoding)) {
	case "quoted-printable":
		reader := quotedprintable.NewReader(bytes.NewReader(body))
		return io.ReadAll(reader)
	case "base64":
		cleaned := bytes.TrimRight(bytes.Join(bytes.Fields(body), nil), "=")
		decoded := make([]byte, base64.RawStdEncoding.DecodedLen(len(cleaned)))
		n, err := base64.RawStdEncoding.Decode(decoded, cleaned)
		if err != nil {
			return nil, err
		}
		return decoded[:n], nil
	default:
		return body, nil
	}
}

func decodeHeader(value string) (string, error) {
	decoder := &mime.WordDecoder{CharsetReader: charset.NewReaderLabel}
	decoded, err := decoder.DecodeHeader(value)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(decoded), nil
}

var (
	htmlDropBlocks = regexp.MustCompile(`(?is)<(head|style|script)\b.*?</(head|style|script)\s*>`)
	htmlLineBreaks = regexp.MustCompile(`(?i)<\s*(br|/p|/div|/tr|/li|/h[1-6])\b[^>]*>`)
	htmlTags       = regexp.MustCompile(`<[^>]*>`)
	inlineSpace    = regexp.MustCompile(`[ \t\f\v\x{00a0}]+`)
	blankLines     = regexp.MustCompile(`\s*\n\s*`)
)

// stripHTML reduces an HTML document to its visible text: head, style and
// script blocks are dropped, block ends become line breaks and entities are
// unescaped.
func stripHTML(input string) string {
	text := htmlDropBlocks.ReplaceAllString(input, " ")
	text = htmlLineBreaks.ReplaceAllString(text, "\n")
	text = htmlTags.ReplaceAllString(text, " ")
	text = html.UnescapeString(text)
	text = inlineSpace.ReplaceAllString(text, " ")
	text = blankLines.ReplaceAllString(text, "\n")
	return strings.TrimSpace(text)
}
