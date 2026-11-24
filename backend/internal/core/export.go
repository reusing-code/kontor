package core

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/reusing-code/kontor/backend/internal/httputil"
	"github.com/reusing-code/kontor/backend/internal/middleware"
	"github.com/reusing-code/kontor/backend/internal/module"
	"github.com/reusing-code/kontor/backend/internal/paperless"
	"github.com/reusing-code/kontor/backend/internal/version"
)

const (
	exportFormat        = "kontor-export"
	exportFormatVersion = 2
	// MaxImportBytes caps an uploaded export file.
	MaxImportBytes = 64 << 20
)

// PaperlessLinkStore is the slice of paperless.Store the export orchestrator
// needs; the paperless config (base URL, token) is deliberately not exported.
type PaperlessLinkStore interface {
	ListAllLinks(ctx context.Context, userID string) ([]paperless.Link, error)
	ImportLinks(ctx context.Context, userID string, links []paperless.Link) error
}

// SetPaperlessLinks wires the optional paperless document-link store into
// full export/import.
func (h *Handler) SetPaperlessLinks(store PaperlessLinkStore) {
	h.paperlessLinks = store
}

type paperlessSection struct {
	Links []paperless.Link `json:"links"`
}

type exportEnvelope struct {
	Format        string                     `json:"format"`
	FormatVersion int                        `json:"formatVersion"`
	ExportedAt    time.Time                  `json:"exportedAt"`
	AppVersion    string                     `json:"appVersion"`
	Settings      *UserSettings              `json:"settings,omitempty"`
	Paperless     *paperlessSection          `json:"paperless,omitempty"`
	Modules       map[string]json.RawMessage `json:"modules"`
}

// Export streams all module data of the authenticated user as a JSON
// download in the v2 envelope format.
func (h *Handler) Export(w http.ResponseWriter, r *http.Request) {
	h.export(w, r, h.registry.All())
}

// ExportModule streams a single module's data in the same envelope format.
func (h *Handler) ExportModule(w http.ResponseWriter, r *http.Request) {
	m, ok := h.registry.Get(r.PathValue("module"))
	if !ok {
		httputil.Error(h.logger, w, http.StatusNotFound, "unknown module")
		return
	}
	h.export(w, r, []module.Module{m})
}

func (h *Handler) export(w http.ResponseWriter, r *http.Request, modules []module.Module) {
	ctx := r.Context()
	userID := middleware.GetUserID(ctx)

	envelope := exportEnvelope{
		Format:        exportFormat,
		FormatVersion: exportFormatVersion,
		ExportedAt:    time.Now().UTC(),
		AppVersion:    version.Get().Version,
		Modules:       map[string]json.RawMessage{},
	}

	if len(modules) == len(h.registry.All()) {
		settings, err := h.store.GetSettings(ctx, userID)
		if err != nil {
			httputil.StoreError(h.logger, w, err)
			return
		}
		envelope.Settings = &settings

		if h.paperlessLinks != nil {
			links, err := h.paperlessLinks.ListAllLinks(ctx, userID)
			if err != nil {
				httputil.StoreError(h.logger, w, err)
				return
			}
			if len(links) > 0 {
				envelope.Paperless = &paperlessSection{Links: links}
			}
		}
	}

	for _, m := range modules {
		section, err := m.ExportSection(ctx, userID)
		if err != nil {
			httputil.StoreError(h.logger, w, err)
			return
		}
		envelope.Modules[m.ID()] = section
	}

	suffix := ""
	if len(modules) == 1 {
		suffix = "-" + modules[0].ID()
	}
	filename := fmt.Sprintf("kontor-export%s-%s.json", suffix, envelope.ExportedAt.Format("20060102-150405"))
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", filename))
	httputil.WriteJSON(h.logger, w, http.StatusOK, envelope)
}

// Import restores an export file. Every module present in the file must be
// empty for the user; IDs are preserved verbatim so cross-references survive.
func (h *Handler) Import(w http.ResponseWriter, r *http.Request) {
	h.importEnvelope(w, r, "")
}

// ImportModule restores a single module's section from an export file, which
// may be a full export.
func (h *Handler) ImportModule(w http.ResponseWriter, r *http.Request) {
	moduleID := r.PathValue("module")
	if _, ok := h.registry.Get(moduleID); !ok {
		httputil.Error(h.logger, w, http.StatusNotFound, "unknown module")
		return
	}
	h.importEnvelope(w, r, moduleID)
}

func (h *Handler) importEnvelope(w http.ResponseWriter, r *http.Request, onlyModule string) {
	ctx := r.Context()
	userID := middleware.GetUserID(ctx)

	var envelope exportEnvelope
	if err := httputil.ReadJSONLimit(r, &envelope, MaxImportBytes); err != nil {
		httputil.BodyError(h.logger, w, err, "invalid export file")
		return
	}
	if envelope.Format != exportFormat || envelope.FormatVersion != exportFormatVersion {
		httputil.Error(h.logger, w, http.StatusBadRequest, fmt.Sprintf("unsupported export format, expected %s v%d", exportFormat, exportFormatVersion))
		return
	}
	for id := range envelope.Modules {
		if _, ok := h.registry.Get(id); !ok {
			httputil.Error(h.logger, w, http.StatusBadRequest, fmt.Sprintf("export contains unknown module %q", id))
			return
		}
	}

	// Import in registry order so ledger references resolve against already
	// imported items.
	var targets []module.Module
	for _, m := range h.registry.All() {
		if _, present := envelope.Modules[m.ID()]; !present {
			continue
		}
		if onlyModule != "" && m.ID() != onlyModule {
			continue
		}
		targets = append(targets, m)
	}
	if len(targets) == 0 {
		httputil.Error(h.logger, w, http.StatusBadRequest, "export file contains no importable module data")
		return
	}

	for _, m := range targets {
		enabled, err := h.store.ModuleEnabled(ctx, userID, m.ID())
		if err != nil {
			httputil.StoreError(h.logger, w, err)
			return
		}
		if !enabled {
			httputil.Error(h.logger, w, http.StatusConflict, fmt.Sprintf("enable the %s module before importing its data", m.ID()))
			return
		}
		empty, err := m.IsEmpty(ctx, userID)
		if err != nil {
			httputil.StoreError(h.logger, w, err)
			return
		}
		if !empty {
			httputil.Error(h.logger, w, http.StatusConflict, fmt.Sprintf("import requires the %s module to have no data", m.ID()))
			return
		}
	}

	result := module.NewImportResult()

	for _, m := range targets {
		if err := m.ImportSection(ctx, userID, envelope.Modules[m.ID()], result); err != nil {
			if errors.Is(err, module.ErrInvalidSection) {
				httputil.Error(h.logger, w, http.StatusBadRequest, err.Error())
				return
			}
			httputil.StoreError(h.logger, w, err)
			return
		}
	}

	for _, m := range targets {
		if err := m.PruneDeadLinks(ctx, userID, result); err != nil {
			httputil.StoreError(h.logger, w, err)
			return
		}
	}

	if onlyModule == "" && envelope.Paperless != nil {
		if h.paperlessLinks == nil {
			result.Warnf("paperless links were not imported")
		} else {
			links := make([]paperless.Link, 0, len(envelope.Paperless.Links))
			for _, l := range envelope.Paperless.Links {
				if !paperless.ValidEntityType(l.EntityType) {
					result.Warnf("skipped paperless link with unknown entity type %q", l.EntityType)
					continue
				}
				links = append(links, l)
			}
			if err := h.paperlessLinks.ImportLinks(ctx, userID, links); err != nil {
				httputil.StoreError(h.logger, w, err)
				return
			}
		}
	}

	// Settings are applied last so imported disabled-module preferences
	// cannot interfere with the import itself.
	if onlyModule == "" && envelope.Settings != nil {
		if err := h.store.UpdateSettings(ctx, userID, *envelope.Settings); err != nil {
			httputil.StoreError(h.logger, w, err)
			return
		}
	}

	httputil.WriteJSON(h.logger, w, http.StatusOK, result)
}
