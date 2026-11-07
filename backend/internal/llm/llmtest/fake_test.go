package llmtest

import (
	"encoding/json"
	"errors"
	"testing"

	"github.com/reusing-code/kontor/backend/internal/llm"
)

type result struct {
	Answer string `json:"answer"`
}

func TestFakeQueue(t *testing.T) {
	boom := errors.New("boom")
	f := New(JSON(result{Answer: "42"}), Error(boom))

	got, err := llm.ExtractAs[result](t.Context(), f, llm.Request{Operation: "op", Input: "in"})
	if err != nil || got.Answer != "42" {
		t.Fatalf("first = %+v, %v", got, err)
	}
	if _, err := f.Extract(t.Context(), llm.Request{Input: "second"}); !errors.Is(err, boom) {
		t.Errorf("second err = %v, want boom", err)
	}
	if _, err := f.Extract(t.Context(), llm.Request{}); !errors.Is(err, ErrNoResponse) {
		t.Errorf("third err = %v, want ErrNoResponse", err)
	}

	reqs := f.Requests()
	if len(reqs) != 3 || reqs[0].Operation != "op" || reqs[0].Schema.Name != "result" || reqs[1].Input != "second" {
		t.Errorf("requests = %+v", reqs)
	}
}

func TestFakeFuncAndDisabled(t *testing.T) {
	f := &Fake{Func: func(req llm.Request) (json.RawMessage, error) {
		return json.RawMessage(`{"answer":"` + req.Input + `"}`), nil
	}}
	got, err := llm.ExtractAs[result](t.Context(), f, llm.Request{Input: "echo"})
	if err != nil || got.Answer != "echo" {
		t.Fatalf("got %+v, %v", got, err)
	}

	off := &Fake{Disabled: true}
	if off.Enabled() {
		t.Error("Enabled() = true for disabled fake")
	}
	if _, err := off.Extract(t.Context(), llm.Request{}); !errors.Is(err, llm.ErrDisabled) {
		t.Errorf("err = %v, want ErrDisabled", err)
	}
}
