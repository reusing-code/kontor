package paperless

import (
	"errors"
	"net/url"
	"strings"
	"time"
)

const (
	EntityContract    = "contract"
	EntityPurchase    = "purchase"
	EntityVehicle     = "vehicle"
	EntityCost        = "cost"
	EntityTransaction = "transaction"
)

func ValidEntityType(t string) bool {
	switch t {
	case EntityContract, EntityPurchase, EntityVehicle, EntityCost, EntityTransaction:
		return true
	}
	return false
}

// Config is the per-user paperless-ngx connection. EncryptedToken is persisted
// via storableConfig and never serialized in API responses.
type Config struct {
	BaseURL        string    `json:"baseUrl"`
	EncryptedToken string    `json:"-"`
	CustomFieldID  int       `json:"customFieldId,omitempty"`
	UpdatedAt      time.Time `json:"updatedAt"`
}

type Link struct {
	EntityType string    `json:"entityType"`
	EntityID   string    `json:"entityId"`
	DocumentID int       `json:"documentId"`
	Title      string    `json:"title"`
	EntityURL  string    `json:"entityUrl"`
	CreatedAt  time.Time `json:"createdAt"`
}

type ConfigInput struct {
	BaseURL string `json:"baseUrl"`
	Token   string `json:"token,omitempty"`
}

func (in *ConfigInput) Validate() error {
	in.BaseURL = strings.TrimRight(strings.TrimSpace(in.BaseURL), "/")
	if err := validateHTTPURL(in.BaseURL); err != nil {
		return errors.New("baseUrl must be a valid http(s) URL")
	}
	in.Token = strings.TrimSpace(in.Token)
	return nil
}

type AttachDocument struct {
	ID    int    `json:"id"`
	Title string `json:"title"`
}

type AttachInput struct {
	EntityURL string           `json:"entityUrl"`
	Documents []AttachDocument `json:"documents"`
}

func (in *AttachInput) Validate() error {
	in.EntityURL = strings.TrimSpace(in.EntityURL)
	if err := validateHTTPURL(in.EntityURL); err != nil {
		return errors.New("entityUrl must be a valid absolute http(s) URL")
	}
	if len(in.Documents) == 0 {
		return errors.New("documents must not be empty")
	}
	for i := range in.Documents {
		if in.Documents[i].ID <= 0 {
			return errors.New("document ids must be positive")
		}
		in.Documents[i].Title = strings.TrimSpace(in.Documents[i].Title)
	}
	return nil
}

func validateHTTPURL(raw string) error {
	parsed, err := url.ParseRequestURI(raw)
	if err != nil || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return errors.New("invalid URL")
	}
	return nil
}

type SearchResult struct {
	ID      int    `json:"id"`
	Title   string `json:"title"`
	Created string `json:"created"`
	Snippet string `json:"snippet,omitempty"`
}

type SearchPage struct {
	Count    int            `json:"count"`
	Page     int            `json:"page"`
	PageSize int            `json:"pageSize"`
	Results  []SearchResult `json:"results"`
}

const (
	DefaultInboxTag = "kontor-inbox"
	DefaultDoneTag  = "kontor"
	maxTagNameLen   = 128
)

// PipelineSettings control the per-user draft pipeline. The done tag also
// marks manually linked documents, independent of Enabled.
type PipelineSettings struct {
	Enabled  bool   `json:"enabled"`
	InboxTag string `json:"inboxTag"`
	DoneTag  string `json:"doneTag"`
}

func DefaultPipelineSettings() PipelineSettings {
	return PipelineSettings{InboxTag: DefaultInboxTag, DoneTag: DefaultDoneTag}
}

func (s *PipelineSettings) Validate() error {
	s.InboxTag = strings.TrimSpace(s.InboxTag)
	s.DoneTag = strings.TrimSpace(s.DoneTag)
	switch {
	case s.InboxTag == "" || s.DoneTag == "":
		return errors.New("inboxTag and doneTag must not be empty")
	case len(s.InboxTag) > maxTagNameLen || len(s.DoneTag) > maxTagNameLen:
		return errors.New("tag names must be at most 128 characters")
	case strings.EqualFold(s.InboxTag, s.DoneTag):
		return errors.New("inboxTag and doneTag must differ")
	}
	return nil
}
