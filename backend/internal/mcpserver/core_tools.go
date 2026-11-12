package mcpserver

import (
	"context"
	"errors"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/reusing-code/kontor/backend/internal/mcptool"
	"github.com/reusing-code/kontor/backend/internal/paperless"
	"github.com/reusing-code/kontor/backend/internal/storage"
)

const maxAttachDocuments = 50

type listModulesOut struct {
	Items []ModuleStatus `json:"items"`
}

type searchIn struct {
	Query string `json:"query,omitempty" jsonschema:"full-text query; empty lists the most recent documents"`
	Page  int    `json:"page,omitempty" jsonschema:"page number, starting at 1"`
}

type searchResult struct {
	ID      int    `json:"id"`
	Title   string `json:"title"`
	Created string `json:"created"`
	Snippet string `json:"snippet"`
}

type searchOut struct {
	Count    int            `json:"count"`
	Page     int            `json:"page"`
	PageSize int            `json:"pageSize"`
	Results  []searchResult `json:"results"`
}

type attachDocument struct {
	ID    int    `json:"id" jsonschema:"paperless document ID"`
	Title string `json:"title,omitempty" jsonschema:"document title as shown in Kontor"`
}

type attachIn struct {
	EntityType string           `json:"entityType" jsonschema:"contract, purchase, vehicle, cost or transaction"`
	EntityID   string           `json:"entityId" jsonschema:"UUID of the entity"`
	Documents  []attachDocument `json:"documents" jsonschema:"1 to 50 paperless documents"`
}

type attachOut struct {
	Linked   int      `json:"linked"`
	Warnings []string `json:"warnings"`
}

var attachEntityTypes = []string{"contract", "purchase", "vehicle", "cost", "transaction"}

// RegisterCoreTools adds the tools that belong to no module.
func RegisterCoreTools(d Deps) {
	r := d.Registrar

	mcptool.Add(r, mcptool.Spec{
		Name:        "list_modules",
		Description: "List Kontor's modules and whether the user has them enabled. Tools of disabled modules are hidden.",
		Scope:       mcptool.ScopeRead,
		ReadOnly:    true,
	}, func(ctx context.Context, c mcptool.Call, _ struct{}) (listModulesOut, error) {
		items, err := d.Modules(ctx, c.UserID)
		if err != nil {
			return listModulesOut{}, err
		}
		if items == nil {
			items = []ModuleStatus{}
		}
		return listModulesOut{Items: items}, nil
	})

	mcptool.Add(r, mcptool.Spec{
		Name:        "search_paperless_documents",
		Description: "Search the user's paperless-ngx archive. Returns document IDs, titles and snippets to use with attach_paperless_documents or as draft context.",
		Scope:       mcptool.ScopeRead,
		ReadOnly:    true,
	}, func(ctx context.Context, c mcptool.Call, in searchIn) (searchOut, error) {
		if in.Page < 0 {
			return searchOut{}, mcptool.Errorf(mcptool.CodeInvalidInput, "page must be at least 1")
		}
		if err := mcptool.CheckLen("query", in.Query, 0, 500); err != nil {
			return searchOut{}, err
		}
		page := max(in.Page, 1)
		client, _, err := d.Paperless.Client(ctx, c.UserID)
		if errors.Is(err, paperless.ErrNotConfigured) {
			return searchOut{}, mcptool.Errorf(mcptool.CodePaperlessNotConfigured, "the user has not connected paperless")
		}
		if err != nil {
			return searchOut{}, err
		}
		res, err := client.SearchDocuments(ctx, in.Query, page)
		if err != nil {
			d.Registrar.Logger().Error("mcp paperless search failed", "tokenId", c.TokenID, "error", err)
			return searchOut{}, mcptool.Errorf(mcptool.CodePaperlessUnavailable, "paperless did not answer the search")
		}
		out := searchOut{Count: res.Count, Page: res.Page, PageSize: res.PageSize, Results: make([]searchResult, 0, len(res.Results))}
		for _, x := range res.Results {
			out.Results = append(out.Results, searchResult{ID: x.ID, Title: x.Title, Created: x.Created, Snippet: x.Snippet})
		}
		return out, nil
	})

	mcptool.Add(r, mcptool.Spec{
		Name: "attach_paperless_documents",
		Description: "Link paperless documents to an existing Kontor entity (contract, purchase, vehicle, cost entry or ledger transaction). " +
			"Kontor writes a back-link to the entity into the paperless document and tags it as linked. Needs PUBLIC_URL to be configured on the server.",
		Scope: mcptool.ScopeLinks,
	}, func(ctx context.Context, c mcptool.Call, in attachIn) (attachOut, error) {
		return attach(ctx, d, c, in)
	})
}

func attach(ctx context.Context, d Deps, c mcptool.Call, in attachIn) (attachOut, error) {
	if !contains(attachEntityTypes, in.EntityType) {
		return attachOut{}, mcptool.Errorf(mcptool.CodeInvalidInput, "entityType must be one of %s", strings.Join(attachEntityTypes, ", "))
	}
	entityID, err := uuid.Parse(in.EntityID)
	if err != nil {
		return attachOut{}, mcptool.Errorf(mcptool.CodeInvalidInput, "entityId must be a UUID")
	}
	if len(in.Documents) == 0 || len(in.Documents) > maxAttachDocuments {
		return attachOut{}, mcptool.Errorf(mcptool.CodeInvalidInput, "documents must contain 1 to %d entries", maxAttachDocuments)
	}
	for i := range in.Documents {
		doc := &in.Documents[i]
		doc.Title = strings.TrimSpace(doc.Title)
		if doc.ID <= 0 {
			return attachOut{}, mcptool.Errorf(mcptool.CodeInvalidInput, "document ids must be positive")
		}
		if utf8.RuneCountInString(doc.Title) > mcptool.MaxTitleLength {
			return attachOut{}, mcptool.Errorf(mcptool.CodeInvalidInput, "document titles must be at most %d characters", mcptool.MaxTitleLength)
		}
	}
	if d.PublicURL == "" {
		return attachOut{}, mcptool.Errorf(mcptool.CodePublicURLNotConfigured, "the server has no PUBLIC_URL, so back-links cannot be written; ask the operator to set it")
	}

	ref, err := d.Entities(ctx, c.UserID, in.EntityType, entityID)
	if errors.Is(err, storage.ErrNotFound) {
		return attachOut{}, mcptool.Errorf(mcptool.CodeNotFound, "no %s with this id", in.EntityType)
	}
	if err != nil {
		return attachOut{}, err
	}
	enabled, err := d.Registrar.ModuleEnabled(ctx, c.UserID, ref.ModuleID)
	if err != nil {
		return attachOut{}, err
	}
	if !enabled {
		return attachOut{}, mcptool.Errorf(mcptool.CodeModuleDisabled, "the %s module is disabled", ref.ModuleID)
	}
	if _, _, err := d.Paperless.Client(ctx, c.UserID); errors.Is(err, paperless.ErrNotConfigured) {
		return attachOut{}, mcptool.Errorf(mcptool.CodePaperlessNotConfigured, "the user has not connected paperless")
	} else if err != nil {
		return attachOut{}, err
	}

	now := time.Now().UTC()
	links := make([]paperless.Link, 0, len(in.Documents))
	for _, doc := range in.Documents {
		links = append(links, paperless.Link{
			EntityType: in.EntityType,
			EntityID:   entityID.String(),
			DocumentID: doc.ID,
			Title:      doc.Title,
			EntityURL:  d.PublicURL + ref.Path,
			CreatedAt:  now,
		})
	}
	warnings, err := d.Paperless.Attach(ctx, c.UserID, links, "")
	if err != nil {
		return attachOut{}, err
	}
	if warnings == nil {
		warnings = []string{}
	}
	return attachOut{Linked: len(links), Warnings: warnings}, nil
}

func contains(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}
