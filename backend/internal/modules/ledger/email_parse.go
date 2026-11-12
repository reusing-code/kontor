package ledger

import (
	"bufio"
	"bytes"
	"encoding/base64"
	"fmt"
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

func parseMessage(r io.Reader) (ParsedEmail, error) {
	msg, err := mail.ReadMessage(bufio.NewReader(r))
	if err != nil {
		return ParsedEmail{}, fmt.Errorf("reading message: %w", err)
	}
	subject := strings.TrimSpace(msg.Header.Get("Subject"))
	if decodedSubject, err := decodeHeader(subject); err == nil && decodedSubject != "" {
		subject = decodedSubject
	}
	from := strings.TrimSpace(msg.Header.Get("From"))
	if decodedFrom, err := decodeHeader(from); err == nil && decodedFrom != "" {
		from = decodedFrom
	}
	parsed := ParsedEmail{
		MessageID: strings.TrimSpace(msg.Header.Get("Message-Id")),
		From:      from,
		Subject:   subject,
	}
	if dateHeader := strings.TrimSpace(msg.Header.Get("Date")); dateHeader != "" {
		if parsedDate, err := mail.ParseDate(dateHeader); err == nil {
			parsed.Date = parsedDate.UTC()
		}
	}
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

func stripHTML(input string) string {
	replacer := strings.NewReplacer("<br>", "\n", "<br/>", "\n", "<br />", "\n", "</p>", "\n", "</div>", "\n")
	cleaned := replacer.Replace(input)
	tagPattern := regexp.MustCompile(`<[^>]+>`)
	cleaned = tagPattern.ReplaceAllString(cleaned, " ")
	spacePattern := regexp.MustCompile(`[ \t\f\v]+`)
	cleaned = spacePattern.ReplaceAllString(cleaned, " ")
	linePattern := regexp.MustCompile(`\n\s+`)
	cleaned = linePattern.ReplaceAllString(cleaned, "\n")
	return strings.TrimSpace(cleaned)
}
