package server

import "github.com/reusing-code/kontor/backend/internal/middleware"

// routeRules says what a personal access token needs for every route on the
// API mux, keyed by the registered pattern. A pattern missing here is denied
// for tokens (default deny); TestRouteRulesCoverRoutes keeps the table and
// the registered routes in sync.
//
//   - RuleRead: every GET, including the exports.
//   - RuleDrafts: the three draft mutations.
//   - RuleLinks: attaching paperless documents and adding a ledger transaction reference.
//   - RuleDenied: every other mutation (entity CRUD, settings, imports, paperless config, quick add, ...).
//   - RuleJWTOnly: token management and the password change; /api/v1/auth/* is
//     public and outside this mux.
var routeRules = map[string]middleware.Rule{
	// JWT only
	"DELETE /api/v1/tokens/{id}":    middleware.RuleJWTOnly,
	"GET /api/v1/tokens":            middleware.RuleJWTOnly,
	"POST /api/v1/tokens":           middleware.RuleJWTOnly,
	"PUT /api/v1/settings/password": middleware.RuleJWTOnly,

	// drafts scope
	"POST /api/v1/drafts/{id}/accept": middleware.RuleDrafts,
	"POST /api/v1/drafts/{id}/reject": middleware.RuleDrafts,
	"PUT /api/v1/drafts/{id}":         middleware.RuleDrafts,

	// links scope
	"POST /api/v1/paperless/links/{entityType}/{entityId}":        middleware.RuleLinks,
	"POST /api/v1/ledger/transactions/{transactionId}/references": middleware.RuleLinks,

	// read scope
	"GET /api/v1/capabilities":                                            middleware.RuleRead,
	"GET /api/v1/categories/{id}/contracts":                               middleware.RuleRead,
	"GET /api/v1/categories/{id}/purchases":                               middleware.RuleRead,
	"GET /api/v1/contracts":                                               middleware.RuleRead,
	"GET /api/v1/contracts/{id}":                                          middleware.RuleRead,
	"GET /api/v1/contracts/summary":                                       middleware.RuleRead,
	"GET /api/v1/contracts/upcoming-renewals":                             middleware.RuleRead,
	"GET /api/v1/costs/{id}":                                              middleware.RuleRead,
	"GET /api/v1/drafts":                                                  middleware.RuleRead,
	"GET /api/v1/drafts/{id}":                                             middleware.RuleRead,
	"GET /api/v1/export":                                                  middleware.RuleRead,
	"GET /api/v1/ledger/accounts":                                         middleware.RuleRead,
	"GET /api/v1/ledger/accounts/{accountId}":                             middleware.RuleRead,
	"GET /api/v1/ledger/accounts/{accountId}/transactions":                middleware.RuleRead,
	"GET /api/v1/ledger/categories":                                       middleware.RuleRead,
	"GET /api/v1/ledger/categories/{id}":                                  middleware.RuleRead,
	"GET /api/v1/ledger/email-accounts":                                   middleware.RuleRead,
	"GET /api/v1/ledger/email-accounts/{emailAccountId}":                  middleware.RuleRead,
	"GET /api/v1/ledger/email-orders":                                     middleware.RuleRead,
	"GET /api/v1/ledger/email-orders/{emailOrderId}":                      middleware.RuleRead,
	"GET /api/v1/ledger/email-orders/{emailOrderId}/candidates":           middleware.RuleRead,
	"GET /api/v1/ledger/imports":                                          middleware.RuleRead,
	"GET /api/v1/ledger/transactions":                                     middleware.RuleRead,
	"GET /api/v1/ledger/transactions/{transactionId}":                     middleware.RuleRead,
	"GET /api/v1/ledger/transactions/{transactionId}/transfer-candidates": middleware.RuleRead,
	"GET /api/v1/modules":                                                 middleware.RuleRead,
	"GET /api/v1/modules/{module}/categories":                             middleware.RuleRead,
	"GET /api/v1/modules/{module}/categories/{id}":                        middleware.RuleRead,
	"GET /api/v1/modules/{module}/export":                                 middleware.RuleRead,
	"GET /api/v1/paperless/config":                                        middleware.RuleRead,
	"GET /api/v1/paperless/documents/{documentId}/thumb":                  middleware.RuleRead,
	"GET /api/v1/paperless/links/{entityType}/{entityId}":                 middleware.RuleRead,
	"GET /api/v1/paperless/pipeline":                                      middleware.RuleRead,
	"GET /api/v1/paperless/search":                                        middleware.RuleRead,
	"GET /api/v1/purchases":                                               middleware.RuleRead,
	"GET /api/v1/purchases/{id}":                                          middleware.RuleRead,
	"GET /api/v1/purchases/summary":                                       middleware.RuleRead,
	"GET /api/v1/settings":                                                middleware.RuleRead,
	"GET /api/v1/vehicles":                                                middleware.RuleRead,
	"GET /api/v1/vehicles/{id}":                                           middleware.RuleRead,
	"GET /api/v1/vehicles/{id}/costs":                                     middleware.RuleRead,
	"GET /api/v1/vehicles/{id}/summary":                                   middleware.RuleRead,

	// never reachable with a token
	"DELETE /api/v1/contracts/{id}":                                       middleware.RuleDenied,
	"DELETE /api/v1/costs/{id}":                                           middleware.RuleDenied,
	"DELETE /api/v1/ledger/categories/{id}":                               middleware.RuleDenied,
	"DELETE /api/v1/ledger/email-accounts/{emailAccountId}":               middleware.RuleDenied,
	"DELETE /api/v1/ledger/transactions/{transactionId}/transfer-link":    middleware.RuleDenied,
	"DELETE /api/v1/modules/{module}/categories/{id}":                     middleware.RuleDenied,
	"DELETE /api/v1/paperless/config":                                     middleware.RuleDenied,
	"DELETE /api/v1/paperless/links/{entityType}/{entityId}/{documentId}": middleware.RuleDenied,
	"DELETE /api/v1/purchases/{id}":                                       middleware.RuleDenied,
	"DELETE /api/v1/vehicles/{id}":                                        middleware.RuleDenied,
	"POST /api/v1/categories/{id}/contracts":                              middleware.RuleDenied,
	"POST /api/v1/categories/{id}/purchases":                              middleware.RuleDenied,
	"POST /api/v1/contracts/import":                                       middleware.RuleDenied,
	"POST /api/v1/import":                                                 middleware.RuleDenied,
	"POST /api/v1/ledger/categories":                                      middleware.RuleDenied,
	"POST /api/v1/ledger/email-accounts":                                  middleware.RuleDenied,
	"POST /api/v1/ledger/email-accounts/{emailAccountId}/scan":            middleware.RuleDenied,
	"POST /api/v1/ledger/email-accounts/{emailAccountId}/test":            middleware.RuleDenied,
	"POST /api/v1/ledger/email-orders/{emailOrderId}/link":                middleware.RuleDenied,
	"POST /api/v1/ledger/email-orders/{emailOrderId}/reject":              middleware.RuleDenied,
	"POST /api/v1/ledger/imports/preview":                                 middleware.RuleDenied,
	"POST /api/v1/ledger/imports/{previewId}/commit":                      middleware.RuleDenied,
	"POST /api/v1/ledger/transactions/{transactionId}/review":             middleware.RuleDenied,
	"POST /api/v1/ledger/transactions/{transactionId}/transfer-link":      middleware.RuleDenied,
	"POST /api/v1/modules/{module}/categories":                            middleware.RuleDenied,
	"POST /api/v1/modules/{module}/import":                                middleware.RuleDenied,
	"POST /api/v1/paperless/config/test":                                  middleware.RuleDenied,
	"POST /api/v1/paperless/pipeline/run":                                 middleware.RuleDenied,
	"POST /api/v1/quick-add":                                              middleware.RuleDenied,
	"POST /api/v1/vehicles":                                               middleware.RuleDenied,
	"POST /api/v1/vehicles/{id}/costs":                                    middleware.RuleDenied,
	"PUT /api/v1/contracts/{id}":                                          middleware.RuleDenied,
	"PUT /api/v1/costs/{id}":                                              middleware.RuleDenied,
	"PUT /api/v1/ledger/categories/{id}":                                  middleware.RuleDenied,
	"PUT /api/v1/ledger/email-accounts/{emailAccountId}":                  middleware.RuleDenied,
	"PUT /api/v1/ledger/transactions/{transactionId}":                     middleware.RuleDenied,
	"PUT /api/v1/modules/{module}/categories/{id}":                        middleware.RuleDenied,
	"PUT /api/v1/paperless/config":                                        middleware.RuleDenied,
	"PUT /api/v1/paperless/pipeline":                                      middleware.RuleDenied,
	"PUT /api/v1/purchases/{id}":                                          middleware.RuleDenied,
	"PUT /api/v1/settings":                                                middleware.RuleDenied,
	"PUT /api/v1/vehicles/{id}":                                           middleware.RuleDenied,
}
