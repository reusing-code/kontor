package ledger

import (
	"os"
	"strings"
	"testing"
)

func TestParseHeader_MatchesParseMessage(t *testing.T) {
	for _, name := range []string{"AmazonDE-1.eml", "PaypalDE-2.eml"} {
		t.Run(name, func(t *testing.T) {
			raw, err := os.ReadFile(fixturePath(name))
			if err != nil {
				t.Fatalf("read fixture: %v", err)
			}
			full := parseFixtureEmail(t, name)
			header, err := parseHeader(headerBlock(raw))
			if err != nil {
				t.Fatalf("parseHeader: %v", err)
			}
			if header.MessageID != full.MessageID || header.From != full.From || header.Subject != full.Subject || !header.Date.Equal(full.Date) {
				t.Errorf("header = %+v, full = %+v", header, full)
			}
			if header.MessageID == "" || header.Date.IsZero() {
				t.Errorf("fixture header incomplete: %+v", header)
			}
			if header.TextBody != "" || header.HTMLBody != "" {
				t.Error("parseHeader returned a body")
			}
		})
	}
}

func TestParseHeader_ErrorIsFixedText(t *testing.T) {
	_, err := parseHeader([]byte("this is not a header SECRET-SUBJECT"))
	if err == nil {
		t.Fatal("want error")
	}
	if strings.Contains(err.Error(), "SECRET") {
		t.Errorf("error quotes input: %v", err)
	}
}

func TestHeaderBlock(t *testing.T) {
	tests := []struct{ name, in, want string }{
		{"crlf", "A: 1\r\nB: 2\r\n\r\nbody\r\n", "A: 1\r\nB: 2"},
		{"lf", "A: 1\nB: 2\n\nbody\n", "A: 1\nB: 2"},
		{"header only", "A: 1\r\nB: 2\r\n\r\n", "A: 1\r\nB: 2"},
		{"no blank line", "A: 1\r\nB: 2\r\n", "A: 1\r\nB: 2"},
		{"blank in body kept out", "A: 1\n\nx\n\ny", "A: 1"},
	}
	for _, tt := range tests {
		if got := string(headerBlock([]byte(tt.in))); got != tt.want {
			t.Errorf("%s: headerBlock = %q, want %q", tt.name, got, tt.want)
		}
	}
}

func TestEmailDedupKey(t *testing.T) {
	if got := emailDedupKey("  <id@x>  ", []byte("ignored")); got != "<id@x>" {
		t.Errorf("message id key = %q", got)
	}
	crlf := emailDedupKey("", []byte("From: a\r\nSubject: s\r\n"))
	lf := emailDedupKey("", []byte("From: a\nSubject: s"))
	other := emailDedupKey("", []byte("From: b\nSubject: s"))
	if !strings.HasPrefix(crlf, "nomid:") || len(crlf) != len("nomid:")+32 {
		t.Errorf("fallback key = %q", crlf)
	}
	if crlf != lf {
		t.Errorf("CRLF and LF variants differ: %q vs %q", crlf, lf)
	}
	if crlf == other {
		t.Error("different headers share a key")
	}
}

func TestEmailDedupKey_BoundsAndCleansMessageIDs(t *testing.T) {
	short := "<abc@example.com>"
	if got := emailDedupKey(short, nil); got != short {
		t.Errorf("short id changed: %q", got)
	}
	long := emailDedupKey("<"+strings.Repeat("a", 70000)+">", nil)
	if !strings.HasPrefix(long, "mid:") || len(long) != len("mid:")+32 {
		t.Errorf("long id key = %q", long)
	}
	ctrl := emailDedupKey("<a\x00b\nc@example.com>", nil)
	if !strings.HasPrefix(ctrl, "mid:") {
		t.Errorf("control character id key = %q", ctrl)
	}
	if emailDedupKey("<"+strings.Repeat("a", 70000)+">", nil) != long || emailDedupKey("<x"+strings.Repeat("a", 70000)+">", nil) == long {
		t.Error("hashed keys are not stable/distinct")
	}
}
