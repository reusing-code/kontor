package paperless

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/reusing-code/kontor/backend/internal/cryptoutil"
	"github.com/reusing-code/kontor/backend/internal/storage"
)

var (
	ErrNotConfigured   = errors.New("paperless is not configured")
	ErrNoEncryptionKey = errors.New("ENCRYPTION_KEY is not configured")
	ErrTokenDecrypt    = errors.New("could not decrypt stored paperless token")
)

// Service holds the paperless operations shared by the HTTP handlers, the
// draft pipeline and the draft accept/reject hooks.
type Service struct {
	store         *Store
	logger        *slog.Logger
	encryptionKey []byte
}

func NewService(store *Store, logger *slog.Logger, encryptionKeyRaw string) *Service {
	key, err := cryptoutil.NormalizeEncryptionKey(encryptionKeyRaw)
	if err != nil {
		logger.Warn("paperless integration requires ENCRYPTION_KEY", "error", err)
		key = nil
	}
	return &Service{store: store, logger: logger, encryptionKey: key}
}

func (s *Service) Store() *Store { return s.store }

// Client builds a client from the user's stored config.
func (s *Service) Client(ctx context.Context, userID string) (*Client, Config, error) {
	cfg, err := s.store.GetConfig(ctx, userID)
	if errors.Is(err, storage.ErrNotFound) {
		return nil, Config{}, ErrNotConfigured
	}
	if err != nil {
		return nil, Config{}, err
	}
	if len(s.encryptionKey) == 0 {
		return nil, Config{}, ErrNoEncryptionKey
	}
	token, err := cryptoutil.DecryptString(cfg.EncryptedToken, s.encryptionKey)
	if err != nil {
		return nil, Config{}, ErrTokenDecrypt
	}
	return NewClient(cfg.BaseURL, token), cfg, nil
}

func (s *Service) ensureCustomField(ctx context.Context, userID string, client *Client, cfg Config) (int, error) {
	if cfg.CustomFieldID > 0 {
		return cfg.CustomFieldID, nil
	}
	fieldID, err := client.EnsureCustomField(ctx)
	if err != nil {
		return 0, err
	}
	cfg.CustomFieldID = fieldID
	if err := s.store.PutConfig(ctx, userID, cfg); err != nil {
		s.logger.Warn("caching paperless custom field id", "error", err)
	}
	return fieldID, nil
}

// Attach stores the links, then best effort writes the back-links, adds the
// done tag and removes removeTag (if set) on the linked documents. Only
// storing the links can fail; paperless problems come back as warnings.
func (s *Service) Attach(ctx context.Context, userID string, links []Link, removeTag string) ([]string, error) {
	for _, l := range links {
		if err := s.store.PutLink(ctx, userID, l); err != nil {
			return nil, err
		}
	}
	warnings := []string{}
	client, cfg, err := s.Client(ctx, userID)
	if err != nil {
		return append(warnings, fmt.Sprintf("paperless is unavailable, back-links and tags were not updated: %v", err)), nil
	}

	fieldID, err := s.ensureCustomField(ctx, userID, client, cfg)
	if err != nil {
		warnings = append(warnings, fmt.Sprintf("could not prepare the %s custom field in paperless: %v", CustomFieldName, err))
	} else {
		for _, l := range links {
			if err := client.SetCustomField(ctx, l.DocumentID, fieldID, l.EntityURL); err != nil {
				s.logger.Warn("setting paperless back-link", "documentId", l.DocumentID, "error", err)
				warnings = append(warnings, fmt.Sprintf("could not set the back-link on document %d: %v", l.DocumentID, err))
			}
		}
	}

	if err := s.tagLinked(ctx, userID, client, documentIDs(links), removeTag); err != nil {
		s.logger.Warn("updating paperless tags", "error", err)
		warnings = append(warnings, fmt.Sprintf("could not update the paperless tags: %v", err))
	}
	return warnings, nil
}

func documentIDs(links []Link) []int {
	ids := make([]int, 0, len(links))
	for _, l := range links {
		ids = append(ids, l.DocumentID)
	}
	return ids
}

func (s *Service) tagLinked(ctx context.Context, userID string, client *Client, docIDs []int, removeTag string) error {
	settings, err := s.store.GetPipelineSettings(ctx, userID)
	if err != nil {
		return err
	}
	doneID, err := client.EnsureTag(ctx, settings.DoneTag)
	if err != nil {
		return err
	}
	var remove []int
	if removeTag != "" {
		id, found, err := client.FindTag(ctx, removeTag)
		if err != nil {
			return err
		}
		if found {
			remove = []int{id}
		}
	}
	return client.ModifyTags(ctx, docIDs, []int{doneID}, remove)
}

// RemoveTag removes a tag from a document if the tag exists.
func (s *Service) RemoveTag(ctx context.Context, userID string, documentID int, tag string) error {
	client, _, err := s.Client(ctx, userID)
	if err != nil {
		return err
	}
	id, found, err := client.FindTag(ctx, tag)
	if err != nil || !found {
		return err
	}
	return client.ModifyTags(ctx, []int{documentID}, nil, []int{id})
}

// Detach deletes a link, then best effort clears the back-link (only if it
// still points at this entity) and removes the done tag once no entity links
// the document anymore.
func (s *Service) Detach(ctx context.Context, userID, entityType, entityID string, documentID int) error {
	deleted, err := s.store.DeleteLink(ctx, userID, entityType, entityID, documentID)
	if err != nil {
		return err
	}
	client, cfg, err := s.Client(ctx, userID)
	if err != nil {
		return nil
	}
	if cfg.CustomFieldID > 0 {
		if err := client.ClearCustomFieldIfMatches(ctx, documentID, cfg.CustomFieldID, deleted.EntityURL); err != nil {
			s.logger.Warn("clearing paperless back-link", "documentId", documentID, "error", err)
		}
	}
	stillLinked, err := s.store.HasDocumentLinks(ctx, userID, documentID)
	if err != nil {
		s.logger.Warn("checking remaining paperless links", "documentId", documentID, "error", err)
		return nil
	}
	if stillLinked {
		return nil
	}
	settings, err := s.store.GetPipelineSettings(ctx, userID)
	if err == nil {
		err = s.RemoveTag(ctx, userID, documentID, settings.DoneTag)
	}
	if err != nil {
		s.logger.Warn("removing paperless done tag", "documentId", documentID, "error", err)
	}
	return nil
}
