// Package llm extracts structured data from text through the server-wide
// OpenAI-compatible LLM endpoint. Prompts and model output are user data:
// they are never logged and never part of error messages.
package llm

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

var (
	ErrDisabled      = errors.New("llm: disabled")
	ErrInvalidOutput = errors.New("llm: invalid output")
	ErrRefused       = errors.New("llm: request refused")
	ErrTruncated     = errors.New("llm: output truncated")
)

// APIError is a non-2xx response from the endpoint. Message is the upstream
// error.message only, truncated.
type APIError struct {
	StatusCode int
	Message    string
}

func (e *APIError) Error() string {
	if e.Message == "" {
		return fmt.Sprintf("llm: endpoint returned %d", e.StatusCode)
	}
	return fmt.Sprintf("llm: endpoint returned %d: %s", e.StatusCode, e.Message)
}

// Request describes one extraction. Operation is a short label used only as
// a metrics dimension (e.g. "paperless.classify").
type Request struct {
	Operation string
	System    string
	Input     string
	Schema    Schema

	// validate is set by ExtractAs so that output not decoding into the
	// target type counts as invalid output and is retried by Client.
	validate func(json.RawMessage) error
}

type Extractor interface {
	Extract(ctx context.Context, req Request) (json.RawMessage, error)
	Enabled() bool
}

type Config struct {
	BaseURL string
	Model   string
	APIKey  string
	Timeout time.Duration
}

// New returns a Client, or Disabled when no base URL is configured.
func New(cfg Config) Extractor {
	if cfg.BaseURL == "" {
		return Disabled{}
	}
	return NewClient(cfg)
}

type Disabled struct{}

func (Disabled) Extract(context.Context, Request) (json.RawMessage, error) {
	return nil, ErrDisabled
}

func (Disabled) Enabled() bool { return false }

// ExtractAs runs the extraction and decodes the result into T. Without an
// explicit schema in req, the schema is derived from T via SchemaFor.
func ExtractAs[T any](ctx context.Context, ex Extractor, req Request) (T, error) {
	var zero T
	if req.Schema.Definition == nil {
		schema, err := SchemaFor[T]()
		if err != nil {
			return zero, err
		}
		req.Schema = schema
	}
	req.validate = func(raw json.RawMessage) error {
		var v T
		return decodeOutput(raw, &v)
	}
	raw, err := ex.Extract(ctx, req)
	if err != nil {
		return zero, err
	}
	var out T
	if err := decodeOutput(raw, &out); err != nil {
		return zero, err
	}
	return out, nil
}

// decodeOutput keeps json error details out of the returned error, as syntax
// errors quote the offending input.
func decodeOutput(raw json.RawMessage, v any) error {
	err := json.Unmarshal(raw, v)
	if err == nil {
		return nil
	}
	if typeErr, ok := errors.AsType[*json.UnmarshalTypeError](err); ok {
		return fmt.Errorf("%w: field %q has the wrong type, want %s", ErrInvalidOutput, typeErr.Field, typeErr.Type)
	}
	return fmt.Errorf("%w: not valid JSON", ErrInvalidOutput)
}
