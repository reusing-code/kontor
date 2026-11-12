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
	bodyBytes, err := io.ReadAll(msg.Body)
	if err != nil {
		return ParsedEmail{}, fmt.Errorf("reading body: %w", err)
	}
	mediaType, params, _ := mime.ParseMediaType(msg.Header.Get("Content-Type"))
	if strings.HasPrefix(mediaType, "multipart/") {
		reader := multipart.NewReader(bytes.NewReader(bodyBytes), params["boundary"])
		for {
			part, err := reader.NextPart()
			if err == io.EOF {
				break
			}
			if err != nil {
				return ParsedEmail{}, fmt.Errorf("reading multipart body: %w", err)
			}
			partBytes, err := readMessagePart(part)
			if err != nil {
				return ParsedEmail{}, fmt.Errorf("reading part body: %w", err)
			}
			partType, _, _ := mime.ParseMediaType(part.Header.Get("Content-Type"))
			switch partType {
			case "text/plain":
				parsed.TextBody = string(partBytes)
			case "text/html":
				parsed.HTMLBody = string(partBytes)
			}
		}
	} else if mediaType == "text/html" {
		decodedBody, err := decodeBody(bodyBytes, msg.Header.Get("Content-Transfer-Encoding"))
		if err != nil {
			return ParsedEmail{}, fmt.Errorf("decoding html body: %w", err)
		}
		parsed.HTMLBody = string(decodedBody)
	} else {
		decodedBody, err := decodeBody(bodyBytes, msg.Header.Get("Content-Transfer-Encoding"))
		if err != nil {
			return ParsedEmail{}, fmt.Errorf("decoding text body: %w", err)
		}
		parsed.TextBody = string(decodedBody)
	}
	if parsed.TextBody == "" && parsed.HTMLBody != "" {
		parsed.TextBody = stripHTML(parsed.HTMLBody)
	}
	return parsed, nil
}

func readMessagePart(part *multipart.Part) ([]byte, error) {
	partBytes, err := io.ReadAll(part)
	if err != nil {
		return nil, err
	}
	return decodeBody(partBytes, part.Header.Get("Content-Transfer-Encoding"))
}

func decodeBody(body []byte, transferEncoding string) ([]byte, error) {
	switch strings.ToLower(strings.TrimSpace(transferEncoding)) {
	case "quoted-printable":
		reader := quotedprintable.NewReader(bytes.NewReader(body))
		return io.ReadAll(reader)
	case "base64":
		cleaned := bytes.ReplaceAll(body, []byte("\r"), nil)
		cleaned = bytes.ReplaceAll(cleaned, []byte("\n"), nil)
		decoded := make([]byte, base64.StdEncoding.DecodedLen(len(cleaned)))
		n, err := base64.StdEncoding.Decode(decoded, cleaned)
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
