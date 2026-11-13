package llm

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"maps"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"
)

const (
	maxRetries         = 1
	maxResponseBytes   = 4 << 20
	maxErrorBodyBytes  = 64 << 10
	maxErrorMessageLen = 300
)

// Client calls an OpenAI-compatible chat completions endpoint with a strict
// json_schema response format.
type Client struct {
	baseURL     string
	model       string
	visionModel string
	apiKey      string
	timeout     time.Duration
	http        *http.Client

	// extraBody is merged into every request body; only for the internal
	// eval harness (provider routing, reasoning settings).
	extraBody map[string]any
}

func NewClient(cfg Config) *Client {
	return &Client{
		baseURL:     strings.TrimRight(cfg.BaseURL, "/"),
		model:       cfg.Model,
		visionModel: cfg.VisionModel,
		apiKey:      cfg.APIKey,
		timeout:     cfg.Timeout,
		http:        &http.Client{},
	}
}

func (c *Client) Enabled() bool { return true }

func (c *Client) Extract(ctx context.Context, req Request) (json.RawMessage, error) {
	op := req.Operation
	if op == "" {
		op = "unknown"
	}
	start := time.Now()
	out, err := c.extract(ctx, op, req)
	requestsTotal.WithLabelValues(op, outcomeOf(err)).Inc()
	requestDuration.WithLabelValues(op).Observe(time.Since(start).Seconds())
	return out, err
}

func (c *Client) extract(ctx context.Context, op string, req Request) (json.RawMessage, error) {
	if req.Schema.Name == "" || len(req.Schema.Definition) == 0 {
		return nil, errors.New("llm: request has no schema")
	}
	if len(req.Images) > 0 && c.visionModel == "" {
		return nil, ErrVisionDisabled
	}
	body, err := c.requestBody(req)
	if err != nil {
		return nil, err
	}
	for attempt := 0; ; attempt++ {
		content, err := c.attempt(ctx, op, body)
		if err != nil {
			return nil, err
		}
		out, verr := validateOutput(content, req.validate)
		if verr == nil {
			return out, nil
		}
		if attempt >= maxRetries {
			return nil, verr
		}
		retriesTotal.WithLabelValues(op).Inc()
	}
}

func validateOutput(content json.RawMessage, validate func(json.RawMessage) error) (json.RawMessage, error) {
	trimmed := bytes.TrimSpace(content)
	if len(trimmed) == 0 || trimmed[0] != '{' || !json.Valid(trimmed) {
		return nil, fmt.Errorf("%w: not a JSON object", ErrInvalidOutput)
	}
	if validate != nil {
		if err := validate(trimmed); err != nil {
			return nil, err
		}
	}
	return trimmed, nil
}

// chatMessage content is a string, or a []contentPart for a user message
// with images.
type chatMessage struct {
	Role    string `json:"role"`
	Content any    `json:"content"`
}

type contentPart struct {
	Type     string    `json:"type"`
	Text     string    `json:"text,omitempty"`
	ImageURL *imageURL `json:"image_url,omitempty"`
}

type imageURL struct {
	URL string `json:"url"`
}

func userContent(req Request) any {
	if len(req.Images) == 0 {
		return req.Input
	}
	parts := make([]contentPart, 0, len(req.Images)+1)
	if req.Input != "" {
		parts = append(parts, contentPart{Type: "text", Text: req.Input})
	}
	for _, img := range req.Images {
		url := "data:" + img.MediaType + ";base64," + base64.StdEncoding.EncodeToString(img.Data)
		parts = append(parts, contentPart{Type: "image_url", ImageURL: &imageURL{URL: url}})
	}
	return parts
}

func (c *Client) requestBody(req Request) ([]byte, error) {
	messages := make([]chatMessage, 0, 2)
	if req.System != "" {
		messages = append(messages, chatMessage{Role: "system", Content: req.System})
	}
	messages = append(messages, chatMessage{Role: "user", Content: userContent(req)})
	model := c.model
	if len(req.Images) > 0 {
		model = c.visionModel
	}
	body := map[string]any{
		"model":    model,
		"messages": messages,
		"response_format": map[string]any{
			"type": "json_schema",
			"json_schema": map[string]any{
				"name":   req.Schema.Name,
				"strict": true,
				"schema": req.Schema.Definition,
			},
		},
	}
	maps.Copy(body, c.extraBody)
	data, err := json.Marshal(body)
	if err != nil {
		return nil, errors.New("llm: encoding request body")
	}
	return data, nil
}

type chatResponse struct {
	Choices []struct {
		Message struct {
			Content *string `json:"content"`
			Refusal *string `json:"refusal"`
		} `json:"message"`
		FinishReason string `json:"finish_reason"`
	} `json:"choices"`
	Usage *struct {
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
	} `json:"usage"`
}

func (c *Client) attempt(ctx context.Context, op string, body []byte) (json.RawMessage, error) {
	ctx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("llm: creating request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "application/json")
	if c.apiKey != "" {
		httpReq.Header.Set("Authorization", "Bearer "+c.apiKey)
	}

	resp, err := c.http.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("llm: calling endpoint: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, apiError(resp)
	}

	var cr chatResponse
	if err := json.NewDecoder(io.LimitReader(resp.Body, maxResponseBytes)).Decode(&cr); err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return nil, fmt.Errorf("llm: reading response: %w", ctxErr)
		}
		return nil, errors.New("llm: malformed response from endpoint")
	}
	if cr.Usage != nil {
		tokensTotal.WithLabelValues(op, "prompt").Add(float64(cr.Usage.PromptTokens))
		tokensTotal.WithLabelValues(op, "completion").Add(float64(cr.Usage.CompletionTokens))
	}
	if len(cr.Choices) == 0 {
		return nil, errors.New("llm: response has no choices")
	}
	choice := cr.Choices[0]
	switch {
	case choice.Message.Refusal != nil && *choice.Message.Refusal != "":
		return nil, ErrRefused
	case choice.FinishReason == "content_filter":
		return nil, ErrRefused
	case choice.FinishReason == "length":
		return nil, ErrTruncated
	case choice.Message.Content == nil:
		return json.RawMessage{}, nil
	}
	return json.RawMessage(*choice.Message.Content), nil
}

func apiError(resp *http.Response) error {
	var body struct {
		Error struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	data, _ := io.ReadAll(io.LimitReader(resp.Body, maxErrorBodyBytes))
	_ = json.Unmarshal(data, &body)
	return &APIError{StatusCode: resp.StatusCode, Message: truncate(strings.TrimSpace(body.Error.Message), maxErrorMessageLen)}
}

func truncate(s string, n int) string {
	if utf8.RuneCountInString(s) <= n {
		return s
	}
	return string([]rune(s)[:n]) + "…"
}

func outcomeOf(err error) string {
	var apiErr *APIError
	switch {
	case err == nil:
		return outcomeOK
	case errors.Is(err, ErrInvalidOutput):
		return outcomeInvalidOutput
	case errors.Is(err, ErrRefused):
		return outcomeRefused
	case errors.Is(err, ErrTruncated):
		return outcomeTruncated
	case errors.As(err, &apiErr):
		return outcomeHTTPError
	case errors.Is(err, context.DeadlineExceeded):
		return outcomeTimeout
	case errors.Is(err, context.Canceled):
		return outcomeCanceled
	default:
		return outcomeError
	}
}
