package quickadd

import (
	"errors"
	"io"
	"mime"
	"net/http"
	"slices"
	"unicode"

	"github.com/reusing-code/kontor/backend/internal/extraction"
	"github.com/reusing-code/kontor/backend/internal/llm"
)

const (
	// MaxFileBytes caps an uploaded PDF or image.
	MaxFileBytes = 10 << 20

	maxFormOverhead = 64 << 10
	maxFieldBytes   = 16 << 10
	minPDFTextRunes = 20
	operationImage  = "quickadd.classify_image"
	kindText        = "text"
	kindImage       = "image"
	kindPDF         = "pdf"
	mediaTypePDF    = "application/pdf"
	fieldFile       = "file"
	fieldText       = "text"
	fieldTargetType = "targetType"
)

// Error codes the frontend translates.
const (
	CodeFileTooLarge        = "file_too_large"
	CodeUnsupportedFile     = "unsupported_file"
	CodeVisionNotConfigured = "vision_not_configured"
	CodePDFNoText           = "pdf_no_text"
	CodeFileUnreadable      = "file_unreadable"
)

var imageTypes = []string{"image/jpeg", "image/png", "image/webp"}

// submission is a parsed quick-add request, typed or uploaded.
type submission struct {
	operation  string
	targetType string
	input      extraction.Input
	// kind and size are the only things logged about a request.
	kind string
	size int
}

type requestError struct {
	status int
	code   string
	msg    string
}

func badRequest(msg string) *requestError {
	return &requestError{status: http.StatusBadRequest, msg: msg}
}

func isMultipart(r *http.Request) bool {
	mediaType, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	return err == nil && mediaType == "multipart/form-data"
}

// parseUpload reads the multipart form (file, optional text and targetType)
// and turns the file into classifier input. The type is taken from the
// file's bytes; the client's content type and file name are ignored.
func (h *Handler) parseUpload(w http.ResponseWriter, r *http.Request) (submission, *requestError) {
	r.Body = http.MaxBytesReader(w, r.Body, MaxFileBytes+maxFormOverhead)
	defer r.Body.Close()
	mr, err := r.MultipartReader()
	if err != nil {
		return submission{}, badRequest("invalid request body")
	}

	tooLarge := &requestError{status: http.StatusRequestEntityTooLarge, code: CodeFileTooLarge, msg: "the file is too large"}
	fields := map[string][]byte{}
	for {
		part, err := mr.NextPart()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			if _, ok := errors.AsType[*http.MaxBytesError](err); ok {
				return submission{}, tooLarge
			}
			return submission{}, badRequest("invalid request body")
		}
		name := part.FormName()
		limit := int64(maxFieldBytes)
		switch name {
		case fieldFile:
			limit = MaxFileBytes
		case fieldText, fieldTargetType:
		default:
			return submission{}, badRequest("unknown form field")
		}
		if _, dup := fields[name]; dup {
			return submission{}, badRequest("duplicate form field")
		}
		data, err := io.ReadAll(io.LimitReader(part, limit+1))
		switch {
		case err != nil:
			if _, ok := errors.AsType[*http.MaxBytesError](err); ok {
				return submission{}, tooLarge
			}
			return submission{}, badRequest("invalid request body")
		case int64(len(data)) > limit && name == fieldFile:
			return submission{}, tooLarge
		case int64(len(data)) > limit:
			return submission{}, badRequest("text is too long")
		}
		fields[name] = data
	}

	file := fields[fieldFile]
	if len(file) == 0 {
		return submission{}, badRequest("file is required")
	}
	hint, reqErr := checkText(string(fields[fieldText]), false)
	if reqErr != nil {
		return submission{}, reqErr
	}
	sub := submission{
		targetType: string(fields[fieldTargetType]),
		size:       len(file),
		input:      extraction.Input{Kind: extraction.InputUpload, Hint: hint},
	}
	if reqErr := checkTargetType(sub.targetType); reqErr != nil {
		return submission{}, reqErr
	}

	switch mediaType := http.DetectContentType(file); {
	case mediaType == mediaTypePDF:
		sub.kind, sub.operation = kindPDF, operation
		text, err := pdfText(file)
		if err != nil {
			return submission{}, &requestError{status: http.StatusUnprocessableEntity, code: CodeFileUnreadable, msg: "the PDF could not be read"}
		}
		if countLetters(text) < minPDFTextRunes {
			return submission{}, &requestError{status: http.StatusUnprocessableEntity, code: CodePDFNoText, msg: "the PDF has no text layer"}
		}
		sub.input.Text = text
	case slices.Contains(imageTypes, mediaType):
		sub.kind, sub.operation = kindImage, operationImage
		if !h.vision {
			return submission{}, visionNotConfigured()
		}
		sub.input.Images = []llm.Image{{MediaType: mediaType, Data: file}}
	default:
		return submission{}, &requestError{status: http.StatusUnsupportedMediaType, code: CodeUnsupportedFile, msg: "only PDF, JPEG, PNG and WebP files are supported"}
	}
	return sub, nil
}

func visionNotConfigured() *requestError {
	return &requestError{status: http.StatusConflict, code: CodeVisionNotConfigured, msg: "no vision model is configured"}
}

func countLetters(s string) int {
	n := 0
	for _, r := range s {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			n++
		}
	}
	return n
}
