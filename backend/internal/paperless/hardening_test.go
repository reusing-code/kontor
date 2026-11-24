package paperless

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

const leakMarker = "SECRET-UPSTREAM-BODY"

func TestConfigInput_RefusesQueryFragmentAndUserInfo(t *testing.T) {
	for _, raw := range []string{
		"https://paperless.example.com?x=1",
		"https://paperless.example.com/?",
		"https://paperless.example.com#frag",
		"https://user:pw@paperless.example.com",
		"https://user@paperless.example.com",
	} {
		in := ConfigInput{BaseURL: raw, Token: "t"}
		if err := in.Validate(); err == nil {
			t.Errorf("%q accepted", raw)
		}
	}
	for _, raw := range []string{"https://paperless.example.com", "http://10.0.0.5:8000/paperless/"} {
		in := ConfigInput{BaseURL: raw, Token: "t"}
		if err := in.Validate(); err != nil {
			t.Errorf("%q refused: %v", raw, err)
		}
	}
}

func TestPutConfig_RefusesBaseURLWithQuery(t *testing.T) {
	mux, _ := newHandlerMux(t, testEncryptionKey)
	rec := do(mux, http.MethodPut, "/api/v1/paperless/config", jsonBody(map[string]string{"baseUrl": "https://p.example.com?page_size=1", "token": "t"}))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, body: %s", rec.Code, rec.Body.String())
	}
}

func upstreamServer(t *testing.T, handler http.HandlerFunc) *Client {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	return NewClient(srv.URL, "token")
}

func TestClient_ErrorsCarryNoUpstreamBody(t *testing.T) {
	c := upstreamServer(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = io.WriteString(w, leakMarker)
	})
	err := c.Ping(context.Background())
	if se, ok := errors.AsType[*StatusError](err); !ok || se.Status != http.StatusInternalServerError {
		t.Fatalf("err = %v", err)
	}
	if strings.Contains(err.Error(), leakMarker) || strings.Contains(PublicError(err), leakMarker) {
		t.Errorf("error = %q", err)
	}
}

func TestClient_DoesNotFollowRedirects(t *testing.T) {
	followed := false
	target := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { followed = true }))
	defer target.Close()
	c := upstreamServer(t, func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, target.URL+"/steal", http.StatusFound)
	})
	err := c.Ping(context.Background())
	if se, ok := errors.AsType[*StatusError](err); !ok || se.Status != http.StatusFound {
		t.Fatalf("err = %v", err)
	}
	if followed || strings.Contains(err.Error(), target.URL) {
		t.Errorf("redirect followed = %v, err = %q", followed, err)
	}
}

func TestClient_CapsJSONBodies(t *testing.T) {
	c := upstreamServer(t, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, `{"id":1,"title":"`)
		_, _ = io.Copy(w, io.LimitReader(repeatReader('a'), maxJSONResponseBytes))
		_, _ = io.WriteString(w, `"}`)
	})
	if _, err := c.GetDocument(context.Background(), 1); !errors.Is(err, ErrResponseTooLarge) {
		t.Fatalf("err = %v, want ErrResponseTooLarge", err)
	}
}

func TestClient_CapsThumbnails(t *testing.T) {
	for _, withLength := range []bool{true, false} {
		c := upstreamServer(t, func(w http.ResponseWriter, _ *http.Request) {
			if !withLength {
				w.Header().Set("Transfer-Encoding", "chunked")
				w.(http.Flusher).Flush()
			}
			_, _ = io.Copy(w, io.LimitReader(repeatReader(0xff), maxThumbnailBytes+1))
		})
		body, _, err := c.Thumbnail(context.Background(), 1)
		if err == nil {
			_, err = io.Copy(io.Discard, body)
			body.Close()
		}
		if !errors.Is(err, ErrResponseTooLarge) {
			t.Errorf("content length %v: err = %v, want ErrResponseTooLarge", withLength, err)
		}
	}
}

func TestHandlers_AnswerWithoutUpstreamText(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		status := http.StatusInternalServerError
		if strings.HasPrefix(r.URL.Path, "/api/documents/1/thumb") {
			status = http.StatusForbidden
		}
		w.WriteHeader(status)
		_, _ = io.WriteString(w, leakMarker)
	}))
	defer upstream.Close()
	mux, _ := newHandlerMux(t, testEncryptionKey)
	putConfig(t, mux, upstream.URL, "token")

	for path, want := range map[string]string{
		"/api/v1/paperless/search?query=x":    "paperless answered HTTP 500",
		"/api/v1/paperless/documents/1/thumb": "paperless rejected the API token",
		"/api/v1/paperless/documents/2/thumb": "paperless answered HTTP 500",
	} {
		rec := do(mux, http.MethodGet, path, nil)
		if rec.Code != http.StatusBadGateway || !strings.Contains(rec.Body.String(), want) || strings.Contains(rec.Body.String(), leakMarker) {
			t.Errorf("%s = %d %s", path, rec.Code, rec.Body.String())
		}
	}
	rec := do(mux, http.MethodPost, "/api/v1/paperless/config/test", nil)
	if rec.Code != http.StatusBadGateway || strings.Contains(rec.Body.String(), leakMarker) {
		t.Errorf("test = %d %s", rec.Code, rec.Body.String())
	}

	upstream.Close()
	rec = do(mux, http.MethodPost, "/api/v1/paperless/config/test", nil)
	if rec.Code != http.StatusBadGateway || !strings.Contains(rec.Body.String(), ErrUnreachable.Error()) || strings.Contains(rec.Body.String(), "127.0.0.1") {
		t.Errorf("unreachable = %d %s", rec.Code, rec.Body.String())
	}
}

type repeatReader byte

func (b repeatReader) Read(p []byte) (int, error) {
	for i := range p {
		p[i] = byte(b)
	}
	return len(p), nil
}
