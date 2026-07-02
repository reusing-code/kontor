package handler

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/reusing-code/kontor/backend/internal/middleware"
	"github.com/reusing-code/kontor/backend/internal/model"
)

type contractImportEntry struct {
	Category string `json:"category"`
	model.ContractInput
}

// categoryTranslations maps nameKey to known translations (all lowercase).
// Used during import to match translated category names to existing categories.
var categoryTranslations = map[string][]string{
	"categoryNames.insurance":          {"versicherung", "versicherungen"},
	"categoryNames.banking":            {"banking / portfolios", "bankwesen", "finanzen"},
	"categoryNames.telecommunications": {"telekommunikation"},
}

type importResult struct {
	Created int           `json:"created"`
	Errors  []importError `json:"errors"`
}

type importError struct {
	Row   int    `json:"row"`
	Error string `json:"error"`
}

func (h *Handler) ImportContracts(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())

	if err := r.ParseMultipartForm(10 << 20); err != nil {
		h.errorResponse(w, http.StatusBadRequest, "invalid multipart form")
		return
	}

	file, _, err := r.FormFile("file")
	if err != nil {
		h.errorResponse(w, http.StatusBadRequest, "missing file field")
		return
	}
	defer file.Close()

	data, err := io.ReadAll(file)
	if err != nil {
		h.errorResponse(w, http.StatusBadRequest, "failed to read file")
		return
	}

	var entries []contractImportEntry
	if err := json.Unmarshal(data, &entries); err != nil {
		h.errorResponse(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}

	categories, err := h.store.ListCategories(r.Context(), userID, "contracts")
	if err != nil {
		h.handleStoreError(w, err)
		return
	}
	catByName := make(map[string]uuid.UUID, len(categories))
	for _, c := range categories {
		catByName[strings.ToLower(c.Name)] = c.ID
		if c.NameKey != "" {
			if suffix, ok := strings.CutPrefix(c.NameKey, "categoryNames."); ok {
				if _, exists := catByName[suffix]; !exists {
					catByName[suffix] = c.ID
				}
			}
			if translations, ok := categoryTranslations[c.NameKey]; ok {
				for _, t := range translations {
					if _, exists := catByName[t]; !exists {
						catByName[t] = c.ID
					}
				}
			}
		}
	}

	result := importResult{Errors: []importError{}}
	for i, entry := range entries {
		row := i + 1

		if entry.Category == "" {
			result.Errors = append(result.Errors, importError{Row: row, Error: "category is required"})
			continue
		}
		if err := entry.ContractInput.Validate(); err != nil {
			result.Errors = append(result.Errors, importError{Row: row, Error: err.Error()})
			continue
		}

		catID, ok := catByName[strings.ToLower(entry.Category)]
		if !ok {
			now := time.Now().UTC()
			cat := model.Category{
				ID:        uuid.New(),
				Name:      entry.Category,
				CreatedAt: now,
				UpdatedAt: now,
			}
			if err := h.store.CreateCategory(r.Context(), userID, "contracts", cat); err != nil {
				result.Errors = append(result.Errors, importError{Row: row, Error: fmt.Sprintf("failed to create category: %v", err)})
				continue
			}
			catID = cat.ID
			catByName[strings.ToLower(entry.Category)] = catID
		}

		now := time.Now().UTC()
		bi := entry.BillingInterval
		if bi == "" {
			bi = model.BillingMonthly
		}
		con := model.Contract{
			ID:                      uuid.New(),
			CategoryID:              catID,
			Name:                    entry.Name,
			ProductName:             entry.ProductName,
			Company:                 entry.Company,
			ContractNumber:          entry.ContractNumber,
			CustomerNumber:          entry.CustomerNumber,
			Price:                   entry.Price,
			BillingInterval:         bi,
			StartDate:               entry.StartDate,
			EndDate:                 entry.EndDate,
			MinimumDurationMonths:   entry.MinimumDurationMonths,
			ExtensionDurationMonths: entry.ExtensionDurationMonths,
			NoticePeriodMonths:      entry.NoticePeriodMonths,
			CustomerPortalURL:       entry.CustomerPortalURL,
			PaperlessURL:            entry.PaperlessURL,
			Comments:                entry.Comments,
			CreatedAt:               now,
			UpdatedAt:               now,
		}

		if err := h.store.CreateContract(r.Context(), userID, con); err != nil {
			result.Errors = append(result.Errors, importError{Row: row, Error: fmt.Sprintf("failed to create contract: %v", err)})
			continue
		}
		result.Created++
	}

	h.writeJSON(w, http.StatusOK, result)
}
