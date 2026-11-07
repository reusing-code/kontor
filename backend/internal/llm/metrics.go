package llm

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

const (
	outcomeOK            = "ok"
	outcomeInvalidOutput = "invalid_output"
	outcomeRefused       = "refused"
	outcomeTruncated     = "truncated"
	outcomeHTTPError     = "http_error"
	outcomeTimeout       = "timeout"
	outcomeCanceled      = "canceled"
	outcomeError         = "error"
)

var (
	requestsTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "llm_requests_total",
		Help: "Total number of LLM extractions by outcome.",
	}, []string{"operation", "outcome"})

	requestDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "llm_request_duration_seconds",
		Help:    "LLM extraction duration in seconds, including retries.",
		Buckets: []float64{0.5, 1, 2, 5, 10, 20, 30, 60, 90, 120},
	}, []string{"operation"})

	tokensTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "llm_tokens_total",
		Help: "Total number of LLM tokens reported by the endpoint.",
	}, []string{"operation", "type"})

	retriesTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "llm_retries_total",
		Help: "Total number of LLM extraction retries after invalid output.",
	}, []string{"operation"})
)
