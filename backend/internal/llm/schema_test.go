package llm_test

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/reusing-code/kontor/backend/internal/llm"
	"github.com/reusing-code/kontor/backend/internal/modules/contracts"
	"github.com/reusing-code/kontor/backend/internal/modules/purchases"
)

type schemaDoc struct {
	Type                 any                  `json:"type"`
	Description          string               `json:"description"`
	Enum                 []any                `json:"enum"`
	Items                *schemaDoc           `json:"items"`
	Properties           map[string]schemaDoc `json:"properties"`
	Required             []string             `json:"required"`
	AdditionalProperties *bool                `json:"additionalProperties"`
	AnyOf                []schemaDoc          `json:"anyOf"`
}

func schemaOf[T any](t *testing.T) (llm.Schema, schemaDoc) {
	t.Helper()
	s, err := llm.SchemaFor[T]()
	if err != nil {
		t.Fatalf("SchemaFor: %v", err)
	}
	var doc schemaDoc
	if err := json.Unmarshal(s.Definition, &doc); err != nil {
		t.Fatalf("decoding schema: %v", err)
	}
	return s, doc
}

// assertStrict checks the OpenAI strict-mode rules on every object.
func assertStrict(t *testing.T, path string, d schemaDoc) {
	t.Helper()
	if d.Type == "object" {
		if d.AdditionalProperties == nil || *d.AdditionalProperties {
			t.Errorf("%s: additionalProperties must be false", path)
		}
		if len(d.Required) != len(d.Properties) {
			t.Errorf("%s: required %v does not list all %d properties", path, d.Required, len(d.Properties))
		}
		for _, name := range d.Required {
			if _, ok := d.Properties[name]; !ok {
				t.Errorf("%s: required %q has no property", path, name)
			}
		}
	}
	for name, p := range d.Properties {
		assertStrict(t, path+"."+name, p)
	}
	if d.Items != nil {
		assertStrict(t, path+"[]", *d.Items)
	}
	for _, alt := range d.AnyOf {
		assertStrict(t, path+"|", alt)
	}
}

func typeOf(t *testing.T, d schemaDoc, prop string) any {
	t.Helper()
	p, ok := d.Properties[prop]
	if !ok {
		t.Fatalf("property %q missing", prop)
	}
	return p.Type
}

func nullableType(base string) []any { return []any{base, "null"} }

func TestSchemaForPurchaseInput(t *testing.T) {
	s, doc := schemaOf[purchases.PurchaseInput](t)
	if s.Name != "PurchaseInput" {
		t.Errorf("Name = %q", s.Name)
	}
	assertStrict(t, "$", doc)
	if len(doc.Properties) != 12 {
		t.Errorf("properties = %d, want 12", len(doc.Properties))
	}
	want := map[string]any{
		"itemName":     "string",
		"type":         nullableType("string"),
		"price":        nullableType("number"),
		"purchaseDate": nullableType("string"),
	}
	for prop, typ := range want {
		if got := typeOf(t, doc, prop); !reflect.DeepEqual(got, typ) {
			t.Errorf("%s type = %v, want %v", prop, got, typ)
		}
	}
	if !strings.HasPrefix(string(s.Definition), `{"type":"object","properties":{"type":`) {
		t.Errorf("properties not in struct order: %s", s.Definition)
	}
}

func TestSchemaForContractInput(t *testing.T) {
	_, doc := schemaOf[contracts.ContractInput](t)
	assertStrict(t, "$", doc)
	want := map[string]any{
		"name":                  "string",
		"billingInterval":       "string",
		"startDate":             "string",
		"endDate":               nullableType("string"),
		"price":                 nullableType("number"),
		"minimumDurationMonths": "integer",
	}
	for prop, typ := range want {
		if got := typeOf(t, doc, prop); !reflect.DeepEqual(got, typ) {
			t.Errorf("%s type = %v, want %v", prop, got, typ)
		}
	}
}

type kind string

type lineItem struct {
	Name     string  `json:"name" llm:"desc=Item name, as printed"`
	Quantity int     `json:"quantity"`
	Price    float64 `json:"price"`
}

type base struct {
	Source string `json:"source"`
}

type synthetic struct {
	base
	Kind     kind       `json:"kind" llm:"desc=Document kind,enum=invoice|receipt"`
	Maybe    *kind      `json:"maybe" llm:"enum=a|b"`
	Paid     bool       `json:"paid"`
	Count    uint8      `json:"count,omitempty"`
	Items    []lineItem `json:"items"`
	Tags     []string   `json:"tags,omitempty"`
	Merchant *lineItem  `json:"merchant"`
	Skipped  string     `json:"-"`
	NoTag    string
	private  string
}

func TestSchemaForSynthetic(t *testing.T) {
	_ = synthetic{}.private
	s, doc := schemaOf[synthetic](t)
	assertStrict(t, "$", doc)

	wantOrder := []string{"source", "kind", "maybe", "paid", "count", "items", "tags", "merchant", "NoTag"}
	if !reflect.DeepEqual(doc.Required, wantOrder) {
		t.Errorf("required = %v, want %v", doc.Required, wantOrder)
	}

	k := doc.Properties["kind"]
	if k.Type != "string" || k.Description != "Document kind" || !reflect.DeepEqual(k.Enum, []any{"invoice", "receipt"}) {
		t.Errorf("kind = %+v", k)
	}
	m := doc.Properties["maybe"]
	if !reflect.DeepEqual(m.Type, nullableType("string")) || !reflect.DeepEqual(m.Enum, []any{"a", "b", nil}) {
		t.Errorf("maybe = %+v", m)
	}
	if got := typeOf(t, doc, "paid"); got != "boolean" {
		t.Errorf("paid type = %v", got)
	}
	if got := typeOf(t, doc, "count"); !reflect.DeepEqual(got, nullableType("integer")) {
		t.Errorf("count type = %v", got)
	}
	if got := typeOf(t, doc, "tags"); !reflect.DeepEqual(got, nullableType("array")) {
		t.Errorf("tags type = %v", got)
	}

	items := doc.Properties["items"]
	if items.Type != "array" || items.Items == nil || items.Items.Type != "object" {
		t.Fatalf("items = %+v", items)
	}
	if d := items.Items.Properties["name"].Description; d != "Item name, as printed" {
		t.Errorf("item name description = %q", d)
	}
	if got := items.Items.Properties["price"].Type; got != "number" {
		t.Errorf("item price type = %v", got)
	}

	merchant := doc.Properties["merchant"]
	if len(merchant.AnyOf) != 2 || merchant.AnyOf[0].Type != "object" || merchant.AnyOf[1].Type != "null" {
		t.Errorf("merchant = %+v, want anyOf [object, null]", merchant)
	}
	if strings.Contains(string(s.Definition), "Skipped") || strings.Contains(string(s.Definition), "private") {
		t.Errorf("schema contains skipped fields: %s", s.Definition)
	}
}

type withMap struct {
	M map[string]string `json:"m"`
}

type withAny struct {
	A any `json:"a"`
}

type withTime struct {
	T time.Time `json:"t"`
}

type withBytes struct {
	B []byte `json:"b"`
}

type withStringOpt struct {
	N int `json:"n,string"`
}

type withIntEnum struct {
	N int `json:"n" llm:"enum=1|2"`
}

type withBadTag struct {
	S string `json:"s" llm:"description=x"`
}

type recursive struct {
	Next *recursive `json:"next"`
}

type duplicate struct {
	A string `json:"x"`
	B string `json:"x"` //nolint:govet // duplicate on purpose
}

type empty struct{}

func TestSchemaForUnsupported(t *testing.T) {
	tests := []struct {
		name string
		fn   func() (llm.Schema, error)
	}{
		{"non-struct root", llm.SchemaFor[[]string]},
		{"map", llm.SchemaFor[withMap]},
		{"interface", llm.SchemaFor[withAny]},
		{"time.Time", llm.SchemaFor[withTime]},
		{"bytes", llm.SchemaFor[withBytes]},
		{"string option", llm.SchemaFor[withStringOpt]},
		{"enum on int", llm.SchemaFor[withIntEnum]},
		{"bad llm tag", llm.SchemaFor[withBadTag]},
		{"recursive", llm.SchemaFor[recursive]},
		{"duplicate", llm.SchemaFor[duplicate]},
		{"empty struct", llm.SchemaFor[empty]},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := tt.fn(); err == nil {
				t.Error("expected error")
			}
		})
	}
}
