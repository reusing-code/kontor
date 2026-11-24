package contracts

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func validContractInput() ContractInput {
	return ContractInput{Name: "Internet", StartDate: "2026-01-01"}
}

func TestContractInputValidate(t *testing.T) {
	neg := -1.0
	tests := []struct {
		name string
		set  func(*ContractInput)
		want string
	}{
		{"valid", func(*ContractInput) {}, ""},
		{"blank name", func(in *ContractInput) { in.Name = "  " }, "name is required"},
		{"long name", func(in *ContractInput) { in.Name = strings.Repeat("n", 201) }, "name must be at most 200 characters"},
		{"no start date", func(in *ContractInput) { in.StartDate = "" }, "startDate is required"},
		{"bad start date", func(in *ContractInput) { in.StartDate = "01.01.2026" }, "startDate must be a date in YYYY-MM-DD format"},
		{"bad end date", func(in *ContractInput) { in.EndDate = "soon" }, "endDate must be a date in YYYY-MM-DD format"},
		{"negative price", func(in *ContractInput) { in.Price = &neg }, "price must not be negative"},
		{"too many months", func(in *ContractInput) { in.MinimumDurationMonths = 1201 }, "minimumDurationMonths must be at most 1200"},
		{"long company", func(in *ContractInput) { in.Company = strings.Repeat("c", 201) }, "company must be at most 200 characters"},
		{"long url", func(in *ContractInput) { in.CustomerPortalURL = strings.Repeat("u", 2049) }, "customerPortalUrl must be at most 2048 characters"},
		{"long comments", func(in *ContractInput) { in.Comments = strings.Repeat("x", 10001) }, "comments must be at most 10000 characters"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			in := validContractInput()
			tt.set(&in)
			got := ""
			if err := in.Validate(); err != nil {
				got = err.Error()
			}
			if got != tt.want {
				t.Errorf("Validate = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestContractInputValidate_TrimsName(t *testing.T) {
	in := validContractInput()
	in.Name = "  Internet \t"
	if err := in.Validate(); err != nil || in.Name != "Internet" {
		t.Errorf("Validate = %v, name %q", err, in.Name)
	}
}

func TestCreateContract_RejectsNegativePrice(t *testing.T) {
	env := newTestEnv(t)
	cat := env.addCategory(t, "Cat")
	rec := httptest.NewRecorder()
	body := map[string]any{"name": "X", "startDate": "2026-01-01", "price": -5}
	env.mux.ServeHTTP(rec, httptest.NewRequest("POST", "/api/v1/categories/"+cat.ID.String()+"/contracts", jsonBody(body)))
	if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "price must not be negative") {
		t.Fatalf("status = %d, body %s", rec.Code, rec.Body.String())
	}
	if list, _ := env.store.List(t.Context(), testUserID); len(list) != 0 {
		t.Errorf("stored %d contracts", len(list))
	}
}

func TestCreateContract_StoresTrimmedName(t *testing.T) {
	env := newTestEnv(t)
	cat := env.addCategory(t, "Cat")
	rec := httptest.NewRecorder()
	env.mux.ServeHTTP(rec, httptest.NewRequest("POST", "/api/v1/categories/"+cat.ID.String()+"/contracts", jsonBody(map[string]any{"name": " Phone ", "startDate": "2026-01-01"})))
	if rec.Code != http.StatusCreated || decodeJSON[Contract](t, rec).Name != "Phone" {
		t.Fatalf("status = %d, body %s", rec.Code, rec.Body.String())
	}
}
