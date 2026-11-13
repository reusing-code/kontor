package quickadd

import (
	"bytes"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/reusing-code/kontor/backend/internal/drafts"
	"github.com/reusing-code/kontor/backend/internal/extraction"
	"github.com/reusing-code/kontor/backend/internal/llm"
	"github.com/reusing-code/kontor/backend/internal/llm/llmtest"
	"github.com/reusing-code/kontor/backend/internal/middleware"
)

const secretFileName = "GEHEIM-rechnung.pdf"

// tinyPNG is a 1x1 PNG.
var tinyPNG = []byte("\x89PNG\r\n\x1a\n\x00\x00\x00\rIHDR\x00\x00\x00\x01\x00\x00\x00\x01\x08\x06\x00\x00\x00\x1f\x15\xc4\x89" +
	"\x00\x00\x00\rIDATx\x9cc\xf8\xff\xff?\x00\x05\xfe\x02\xfe\xa75\x81\x84\x00\x00\x00\x00IEND\xaeB`\x82")

var tinyJPEG = []byte("\xff\xd8\xff\xe0\x00\x10JFIF\x00\x01 GEHEIM-IMAGE")

func testdata(t *testing.T, name string) []byte {
	t.Helper()
	data, err := os.ReadFile("testdata/" + name)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

type form struct {
	file       []byte
	text       string
	targetType string
	extra      map[string]string
}

func (env *testEnv) upload(t *testing.T, f form) *httptest.ResponseRecorder {
	t.Helper()
	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	if f.file != nil {
		// The declared name and type are wrong on purpose: only the bytes count.
		part, err := mw.CreateFormFile("file", secretFileName)
		if err != nil {
			t.Fatal(err)
		}
		part.Write(f.file)
	}
	if f.text != "" {
		mw.WriteField("text", f.text)
	}
	if f.targetType != "" {
		mw.WriteField("targetType", f.targetType)
	}
	for k, v := range f.extra {
		mw.WriteField(k, v)
	}
	mw.Close()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/quick-add", &body)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	req = req.WithContext(middleware.SetUserID(req.Context(), "u1"))
	rec := httptest.NewRecorder()
	env.handler.Classify(rec, req)
	return rec
}

func TestPDFText(t *testing.T) {
	for _, name := range []string{"invoice.pdf", "invoice-latex.pdf"} {
		text, err := pdfText(testdata(t, name))
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		// Word gaps, kerning inside a word ("02.11.2026") and a jump to a
		// right-aligned amount ("870 560,00") must all come out right.
		for _, want := range []string{"Rechnung Nr. 2026-4711", "Reifen Müller GmbH", "Rechnungsdatum: 02.11.2026", "Hauptstraße 5", "TS 870", "560,00 €",
			"Gesamtbetrag inkl. 19 % MwSt.: 640,00 €", "Kennzeichen B-XY 123"} {
			if !strings.Contains(text, want) {
				t.Errorf("%s: text lacks %q:\n%s", name, want, text)
			}
		}
		if strings.Contains(text, "870560") {
			t.Errorf("%s: columns run together:\n%s", name, text)
		}
	}

	if text, err := pdfText(testdata(t, "scan.pdf")); err != nil || text != "" {
		t.Errorf("scan = %q, %v; want no text", text, err)
	}
	for name, data := range map[string][]byte{
		"garbage":   []byte("%PDF-1.7\nnot a pdf at all"),
		"truncated": testdata(t, "invoice.pdf")[:2000],
		"empty":     {},
	} {
		if text, err := pdfText(data); err != errPDFUnreadable || text != "" {
			t.Errorf("%s = %q, %v; want errPDFUnreadable", name, text, err)
		}
	}
}

func TestUpload_PDF(t *testing.T) {
	env := newTestEnv()
	env.llm.Push(llmtest.JSON(extraction.Classification{Kind: "cost", Confident: true, Cost: &extraction.CostExtract{
		VehicleID: vehicleID, Type: "tires", Description: "Winter tires", Amount: amount(640), Currency: "EUR", Date: "2026-11-02",
	}}))
	resp := decodeResponse(t, env.upload(t, form{file: testdata(t, "invoice.pdf"), text: "  for the Golf  "}))
	if resp.TargetType != drafts.TargetCost || resp.ParentID != vehicleID || !resp.Confident {
		t.Errorf("response = %+v", resp)
	}

	req := env.llm.Requests()[0]
	if req.Operation != "quickadd.classify" || len(req.Images) != 0 {
		t.Errorf("operation = %q, images = %d", req.Operation, len(req.Images))
	}
	if !strings.HasPrefix(req.Input, "Note from the user: for the Golf\n\nContent:\nRechnung Nr. 2026-4711") ||
		!strings.Contains(req.System, "the user uploaded") {
		t.Errorf("input = %q", req.Input)
	}
	if strings.Contains(req.Input, secretFileName) || strings.Contains(req.System, secretFileName) {
		t.Error("the file name reached the model")
	}
}

func TestUpload_Image(t *testing.T) {
	for name, tt := range map[string]struct {
		file      []byte
		mediaType string
	}{
		"png":  {tinyPNG, "image/png"},
		"jpeg": {tinyJPEG, "image/jpeg"},
		"webp": {[]byte("RIFF\x10\x00\x00\x00WEBPVP8 data"), "image/webp"},
	} {
		t.Run(name, func(t *testing.T) {
			env := newTestEnv()
			env.llm.Push(llmtest.JSON(extraction.Classification{Kind: "purchase", Confident: true, Purchase: &extraction.PurchaseExtract{ItemName: "Drill"}}))
			resp := decodeResponse(t, env.upload(t, form{file: tt.file, targetType: drafts.TargetPurchase}))
			if resp.TargetType != drafts.TargetPurchase {
				t.Errorf("response = %+v", resp)
			}
			req := env.llm.Requests()[0]
			if req.Operation != "quickadd.classify_image" || req.Input != "" || len(req.Images) != 1 ||
				req.Images[0].MediaType != tt.mediaType || !bytes.Equal(req.Images[0].Data, tt.file) {
				t.Errorf("request = %q %q, %d images", req.Operation, req.Input, len(req.Images))
			}
			if !strings.Contains(req.System, `The user has chosen the kind "purchase"`) {
				t.Error("forced target type is not in the prompt")
			}
		})
	}
}

func TestUpload_Errors(t *testing.T) {
	pdf := testdata(t, "invoice.pdf")
	tests := []struct {
		name   string
		setup  func(*testEnv)
		form   form
		status int
		code   string
		calls  int
	}{
		{name: "no file", form: form{text: "x"}, status: http.StatusBadRequest},
		{name: "unknown field", form: form{file: pdf, extra: map[string]string{"foo": "y"}}, status: http.StatusBadRequest},
		{name: "invalid target", form: form{file: pdf, targetType: "none"}, status: http.StatusBadRequest},
		{name: "hint too long", form: form{file: pdf, text: strings.Repeat("ä", MaxTextRunes+1)}, status: http.StatusBadRequest},
		{
			name:  "disabled target",
			setup: func(env *testEnv) { env.enabled[drafts.TargetCost] = false },
			form:  form{file: pdf, targetType: drafts.TargetCost}, status: http.StatusBadRequest,
		},
		{name: "too large", form: form{file: append([]byte("%PDF-1.7\n"), make([]byte, MaxFileBytes)...)}, status: http.StatusRequestEntityTooLarge, code: CodeFileTooLarge},
		{name: "far too large", form: form{file: make([]byte, MaxFileBytes+2*maxFormOverhead)}, status: http.StatusRequestEntityTooLarge, code: CodeFileTooLarge},
		{name: "text file", form: form{file: []byte("Rechnung 640 EUR")}, status: http.StatusUnsupportedMediaType, code: CodeUnsupportedFile},
		{name: "gif", form: form{file: []byte("GIF89a\x01\x00\x01\x00")}, status: http.StatusUnsupportedMediaType, code: CodeUnsupportedFile},
		{name: "scan", form: form{file: testdata(t, "scan.pdf")}, status: http.StatusUnprocessableEntity, code: CodePDFNoText},
		{name: "broken pdf", form: form{file: []byte("%PDF-1.7\nbroken")}, status: http.StatusUnprocessableEntity, code: CodeFileUnreadable},
		{
			name:  "image without vision model",
			setup: func(env *testEnv) { env.handler.vision = false },
			form:  form{file: tinyPNG}, status: http.StatusConflict, code: CodeVisionNotConfigured,
		},
		{
			name:  "vision refused by the client",
			setup: func(env *testEnv) { env.llm.Push(llmtest.Error(llm.ErrVisionDisabled)) },
			form:  form{file: tinyPNG}, status: http.StatusConflict, code: CodeVisionNotConfigured, calls: 1,
		},
		{
			name:  "llm disabled",
			setup: func(env *testEnv) { env.llm.Disabled = true },
			form:  form{file: pdf}, status: http.StatusConflict,
		},
		{
			name:  "llm unavailable",
			setup: func(env *testEnv) { env.llm.Push(llmtest.Error(&llm.APIError{StatusCode: 503})) },
			form:  form{file: tinyPNG}, status: http.StatusBadGateway, calls: 1,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			env := newTestEnv()
			if tt.setup != nil {
				tt.setup(env)
			}
			rec := env.upload(t, tt.form)
			if rec.Code != tt.status {
				t.Fatalf("status = %d, want %d; body %s", rec.Code, tt.status, rec.Body)
			}
			var body map[string]string
			if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil || body["error"] == "" || body["code"] != tt.code {
				t.Errorf("body = %s, want code %q", rec.Body, tt.code)
			}
			if got := len(env.llm.Requests()); got != tt.calls {
				t.Errorf("llm calls = %d, want %d", got, tt.calls)
			}
		})
	}
}

// A PDF without the vision model still works: only images need it.
func TestUpload_PDFWithoutVisionModel(t *testing.T) {
	env := newTestEnv()
	env.handler.vision = false
	env.llm.Push(llmtest.JSON(extraction.Classification{Kind: "none"}))
	if resp := decodeResponse(t, env.upload(t, form{file: testdata(t, "invoice.pdf")})); resp.Note != extraction.NoteUnclassified {
		t.Errorf("response = %+v", resp)
	}
}

func TestUpload_LogsNoContent(t *testing.T) {
	env := newTestEnv()
	env.llm.Push(llmtest.Error(&llm.APIError{StatusCode: 503}), llmtest.Error(&llm.APIError{StatusCode: 503}))
	env.upload(t, form{file: tinyJPEG, text: "GEHEIM-HINT"})
	env.upload(t, form{file: testdata(t, "invoice.pdf"), text: "GEHEIM-HINT"})
	env.upload(t, form{file: []byte("GEHEIM-TEXTFILE")})
	env.upload(t, form{file: []byte("%PDF-1.7\nGEHEIM-BROKEN")})

	logs := env.logs.String()
	for _, secret := range []string{"GEHEIM", "Müller", "4711", "rechnung.pdf"} {
		if strings.Contains(logs, secret) {
			t.Errorf("logs leak %q:\n%s", secret, logs)
		}
	}
	for _, want := range []string{"kind=image", "kind=pdf", "bytes="} {
		if !strings.Contains(logs, want) {
			t.Errorf("logs lack %q:\n%s", want, logs)
		}
	}
}
