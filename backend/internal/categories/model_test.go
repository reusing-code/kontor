package categories

import (
	"strings"
	"testing"
)

func TestCategoryInputValidate(t *testing.T) {
	for _, tt := range []struct{ name, want string }{
		{"Insurance", ""},
		{"   ", "name is required"},
		{strings.Repeat("n", 201), "name must be at most 200 characters"},
	} {
		in := CategoryInput{Name: tt.name}
		got := ""
		if err := in.Validate(); err != nil {
			got = err.Error()
		}
		if got != tt.want {
			t.Errorf("Validate(%q) = %q, want %q", tt.name, got, tt.want)
		}
	}
	in := CategoryInput{Name: " Insurance "}
	if err := in.Validate(); err != nil || in.Name != "Insurance" {
		t.Errorf("trim: %v, %q", err, in.Name)
	}
}
