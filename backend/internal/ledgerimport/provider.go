package ledgerimport

import "io"

type Provider interface {
	SourceType() SourceType
	Parse(r io.Reader) (ParseResult, error)
}
