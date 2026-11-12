package server

import "net/http"

// apiMux records every registered pattern so tests can compare the route set
// with the scope table.
type apiMux struct {
	*http.ServeMux
	patterns []string
}

func newAPIMux() *apiMux { return &apiMux{ServeMux: http.NewServeMux()} }

func (m *apiMux) Handle(pattern string, h http.Handler) {
	m.patterns = append(m.patterns, pattern)
	m.ServeMux.Handle(pattern, h)
}

func (m *apiMux) HandleFunc(pattern string, h func(http.ResponseWriter, *http.Request)) {
	m.Handle(pattern, http.HandlerFunc(h))
}
