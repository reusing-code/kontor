package ledger

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"unicode"
)

// maxMessageIDBytes bounds the Message-ID part of a storage key: the header is
// sender-controlled and Badger rejects keys above 65000 bytes.
const maxMessageIDBytes = 200

// emailDedupKey identifies a message across scans and sources. It does not
// depend on model output: the Message-ID (verbatim when short and printable,
// else its hash), or a hash of the normalised header block when there is none.
func emailDedupKey(messageID string, header []byte) string {
	id := strings.TrimSpace(messageID)
	switch {
	case id == "":
		normalised := strings.TrimSpace(strings.ReplaceAll(string(header), "\r\n", "\n"))
		sum := sha256.Sum256([]byte(normalised))
		return "nomid:" + hex.EncodeToString(sum[:16])
	case len(id) > maxMessageIDBytes || strings.ContainsFunc(id, unicode.IsControl):
		sum := sha256.Sum256([]byte(id))
		return "mid:" + hex.EncodeToString(sum[:16])
	}
	return id
}
