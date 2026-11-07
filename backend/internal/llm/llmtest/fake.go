// Package llmtest provides a scripted llm.Extractor for tests.
package llmtest

import (
	"context"
	"encoding/json"
	"errors"
	"sync"

	"github.com/reusing-code/kontor/backend/internal/llm"
)

var ErrNoResponse = errors.New("llmtest: no scripted response left")

type Response struct {
	Output json.RawMessage
	Err    error
}

// JSON scripts a successful response with v encoded as JSON.
func JSON(v any) Response {
	data, err := json.Marshal(v)
	if err != nil {
		return Response{Err: err}
	}
	return Response{Output: data}
}

func Error(err error) Response {
	return Response{Err: err}
}

// Fake answers from Func when set, otherwise from the queued responses in
// order, and records every request it receives.
type Fake struct {
	Func     func(llm.Request) (json.RawMessage, error)
	Disabled bool

	mu        sync.Mutex
	responses []Response
	requests  []llm.Request
}

var _ llm.Extractor = (*Fake)(nil)

func New(responses ...Response) *Fake {
	return &Fake{responses: responses}
}

func (f *Fake) Push(responses ...Response) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.responses = append(f.responses, responses...)
}

func (f *Fake) Extract(ctx context.Context, req llm.Request) (json.RawMessage, error) {
	f.mu.Lock()
	f.requests = append(f.requests, req)
	f.mu.Unlock()
	switch {
	case f.Disabled:
		return nil, llm.ErrDisabled
	case ctx.Err() != nil:
		return nil, ctx.Err()
	case f.Func != nil:
		return f.Func(req)
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.responses) == 0 {
		return nil, ErrNoResponse
	}
	next := f.responses[0]
	f.responses = f.responses[1:]
	return next.Output, next.Err
}

func (f *Fake) Enabled() bool { return !f.Disabled }

func (f *Fake) Requests() []llm.Request {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]llm.Request(nil), f.requests...)
}
