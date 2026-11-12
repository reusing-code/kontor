package mcptool

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

const (
	outcomeOK       = "ok"
	outcomeToolErr  = "tool_error"
	outcomeForbid   = "forbidden"
	outcomeInternal = "internal_error"
)

var (
	toolCalls = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "mcp_tool_calls_total",
		Help: "MCP tool calls by tool and outcome.",
	}, []string{"tool", "outcome"})

	toolDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "mcp_tool_call_duration_seconds",
		Help:    "MCP tool call duration by tool.",
		Buckets: prometheus.DefBuckets,
	}, []string{"tool"})
)
