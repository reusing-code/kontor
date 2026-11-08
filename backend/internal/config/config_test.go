package config

import (
	"os"
	"strings"
	"testing"
	"time"
)

func TestLoadLLM(t *testing.T) {
	tests := []struct {
		name        string
		env         map[string]string
		wantErr     string
		wantEnabled bool
	}{
		{name: "unset disables", env: map[string]string{}},
		{
			name:        "base url and model enable",
			env:         map[string]string{"LLM_BASE_URL": "http://gw/v1", "LLM_MODEL": "kontor"},
			wantEnabled: true,
		},
		{
			name:        "api key optional",
			env:         map[string]string{"LLM_BASE_URL": "https://gw/v1", "LLM_MODEL": "kontor", "LLM_API_KEY": "k"},
			wantEnabled: true,
		},
		{name: "model without base url", env: map[string]string{"LLM_MODEL": "kontor"}, wantErr: "LLM_BASE_URL is not"},
		{name: "key without base url", env: map[string]string{"LLM_API_KEY": "k"}, wantErr: "LLM_BASE_URL is not"},
		{name: "base url without model", env: map[string]string{"LLM_BASE_URL": "http://gw/v1"}, wantErr: "LLM_MODEL is not"},
		{
			name:    "relative base url",
			env:     map[string]string{"LLM_BASE_URL": "gw/v1", "LLM_MODEL": "kontor"},
			wantErr: "absolute http(s) URL",
		},
		{
			name:    "non-positive timeout",
			env:     map[string]string{"LLM_BASE_URL": "http://gw/v1", "LLM_MODEL": "kontor", "LLM_TIMEOUT": "0s"},
			wantErr: "LLM_TIMEOUT",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("JWT_SECRET", "test")
			for _, k := range []string{"LLM_BASE_URL", "LLM_MODEL", "LLM_API_KEY", "LLM_TIMEOUT"} {
				v, ok := tt.env[k]
				t.Setenv(k, v)
				if !ok {
					os.Unsetenv(k)
				}
			}

			cfg, err := Load()
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("Load() error = %v, want containing %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("Load(): %v", err)
			}
			if cfg.LLMEnabled() != tt.wantEnabled {
				t.Errorf("LLMEnabled() = %v, want %v", cfg.LLMEnabled(), tt.wantEnabled)
			}
			if cfg.LLMTimeout != 60*time.Second {
				t.Errorf("LLMTimeout = %v, want default 60s", cfg.LLMTimeout)
			}
		})
	}
}

func TestLoadFXRatesURL(t *testing.T) {
	tests := []struct {
		name        string
		value       *string
		wantEnabled bool
		wantErr     bool
	}{
		{name: "default", wantEnabled: true},
		{name: "empty falls back to default", value: new(""), wantEnabled: true},
		{name: "off disables", value: new("off")},
		{name: "custom url", value: new("http://fx.local/v1"), wantEnabled: true},
		{name: "relative url", value: new("fx.local"), wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("JWT_SECRET", "test")
			t.Setenv("FX_RATES_URL", "")
			if tt.value == nil {
				os.Unsetenv("FX_RATES_URL")
			} else {
				t.Setenv("FX_RATES_URL", *tt.value)
			}
			cfg, err := Load()
			if tt.wantErr {
				if err == nil || !strings.Contains(err.Error(), "FX_RATES_URL") {
					t.Fatalf("Load() error = %v, want FX_RATES_URL error", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("Load(): %v", err)
			}
			if cfg.FXRatesEnabled() != tt.wantEnabled {
				t.Errorf("FXRatesEnabled() = %v, want %v", cfg.FXRatesEnabled(), tt.wantEnabled)
			}
		})
	}
}
