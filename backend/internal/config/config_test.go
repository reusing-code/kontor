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

func TestLoadPublicURL(t *testing.T) {
	tests := []struct {
		name    string
		value   string
		want    string
		wantErr bool
	}{
		{name: "unset", value: "", want: ""},
		{name: "https", value: "https://kontor.example.com", want: "https://kontor.example.com"},
		{name: "trailing slash trimmed", value: "https://kontor.example.com/", want: "https://kontor.example.com"},
		{name: "path kept", value: "http://host:8080/kontor/", want: "http://host:8080/kontor"},
		{name: "ftp", value: "ftp://kontor.example.com", wantErr: true},
		{name: "relative", value: "kontor.example.com", wantErr: true},
		{name: "no host", value: "https://", wantErr: true},
		{name: "query", value: "https://kontor.example.com?x=1", wantErr: true},
		{name: "empty query marker", value: "https://kontor.example.com/?", wantErr: true},
		{name: "fragment", value: "https://kontor.example.com/#top", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("JWT_SECRET", "test")
			t.Setenv("PUBLIC_URL", tt.value)
			cfg, err := Load()
			if tt.wantErr {
				if err == nil || !strings.Contains(err.Error(), "PUBLIC_URL") {
					t.Fatalf("Load() error = %v", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("Load() error = %v", err)
			}
			if cfg.PublicURL != tt.want {
				t.Fatalf("PublicURL = %q, want %q", cfg.PublicURL, tt.want)
			}
		})
	}
}

func TestLoadLedgerSuggest(t *testing.T) {
	tests := []struct {
		name               string
		batch, max         string
		wantBatch, wantMax int
		wantErr            bool
	}{
		{name: "defaults", wantBatch: 25, wantMax: 100},
		{name: "custom", batch: "10", max: "10", wantBatch: 10, wantMax: 10},
		{name: "batch zero", batch: "0", wantErr: true},
		{name: "batch too large", batch: "101", max: "500", wantErr: true},
		{name: "max below batch", batch: "20", max: "10", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("JWT_SECRET", "test")
			for key, value := range map[string]string{"LEDGER_SUGGEST_BATCH_SIZE": tt.batch, "LEDGER_SUGGEST_MAX_PER_RUN": tt.max} {
				t.Setenv(key, value)
				if value == "" {
					os.Unsetenv(key)
				}
			}
			cfg, err := Load()
			if tt.wantErr {
				if err == nil || !strings.Contains(err.Error(), "LEDGER_SUGGEST_") {
					t.Fatalf("Load() error = %v, want LEDGER_SUGGEST_ error", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("Load(): %v", err)
			}
			if cfg.LedgerSuggestBatchSize != tt.wantBatch || cfg.LedgerSuggestMaxPerRun != tt.wantMax {
				t.Errorf("batch, max = %d, %d, want %d, %d", cfg.LedgerSuggestBatchSize, cfg.LedgerSuggestMaxPerRun, tt.wantBatch, tt.wantMax)
			}
		})
	}
}

func TestLoadEnableBanking(t *testing.T) {
	const key = "0123456789abcdef0123456789abcdef"
	vars := []string{"ENABLE_BANKING_APP_ID", "ENABLE_BANKING_KEY_PATH", "ENABLE_BANKING_API_URL", "ENABLE_BANKING_SYNC_INTERVAL", "ENCRYPTION_KEY", "EMAIL_ENCRYPTION_KEY"}
	tests := []struct {
		name         string
		env          map[string]string
		wantErr      string
		wantEnabled  bool
		wantURL      string
		wantInterval time.Duration
	}{
		{name: "both empty disables", env: map[string]string{}, wantURL: "https://api.enablebanking.com", wantInterval: 24 * time.Hour},
		{
			name:         "enabled with defaults",
			env:          map[string]string{"ENABLE_BANKING_APP_ID": "app", "ENABLE_BANKING_KEY_PATH": "/k.pem", "ENCRYPTION_KEY": key},
			wantEnabled:  true,
			wantURL:      "https://api.enablebanking.com",
			wantInterval: 24 * time.Hour,
		},
		{
			name: "url trailing slash trimmed and zero interval",
			env: map[string]string{
				"ENABLE_BANKING_APP_ID": "app", "ENABLE_BANKING_KEY_PATH": "/k.pem", "ENCRYPTION_KEY": key,
				"ENABLE_BANKING_API_URL": "http://localhost:9/", "ENABLE_BANKING_SYNC_INTERVAL": "0",
			},
			wantEnabled: true,
			wantURL:     "http://localhost:9",
		},
		{name: "only app id", env: map[string]string{"ENABLE_BANKING_APP_ID": "app"}, wantErr: "must be set together"},
		{name: "only key path", env: map[string]string{"ENABLE_BANKING_KEY_PATH": "/k.pem"}, wantErr: "must be set together"},
		{
			name:    "invalid api url",
			env:     map[string]string{"ENABLE_BANKING_API_URL": "api.example.com"},
			wantErr: "ENABLE_BANKING_API_URL",
		},
		{
			name:    "negative interval",
			env:     map[string]string{"ENABLE_BANKING_SYNC_INTERVAL": "-1h"},
			wantErr: "ENABLE_BANKING_SYNC_INTERVAL",
		},
		{
			name:    "missing encryption key",
			env:     map[string]string{"ENABLE_BANKING_APP_ID": "app", "ENABLE_BANKING_KEY_PATH": "/k.pem"},
			wantErr: "ENCRYPTION_KEY",
		},
		{
			name:    "invalid encryption key",
			env:     map[string]string{"ENABLE_BANKING_APP_ID": "app", "ENABLE_BANKING_KEY_PATH": "/k.pem", "ENCRYPTION_KEY": "short"},
			wantErr: "ENCRYPTION_KEY",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("JWT_SECRET", "test")
			for _, k := range vars {
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
				t.Fatalf("Load() error = %v", err)
			}
			if cfg.EnableBankingEnabled() != tt.wantEnabled {
				t.Fatalf("EnableBankingEnabled() = %v, want %v", cfg.EnableBankingEnabled(), tt.wantEnabled)
			}
			if cfg.EnableBankingAPIURL != tt.wantURL {
				t.Fatalf("EnableBankingAPIURL = %q, want %q", cfg.EnableBankingAPIURL, tt.wantURL)
			}
			if cfg.EnableBankingSyncInterval != tt.wantInterval {
				t.Fatalf("EnableBankingSyncInterval = %v, want %v", cfg.EnableBankingSyncInterval, tt.wantInterval)
			}
		})
	}
}

func TestLoadEncryptionKey(t *testing.T) {
	const key = "0123456789abcdef0123456789abcdef"
	const other = "fedcba9876543210fedcba9876543210"
	tests := []struct {
		name           string
		env            map[string]string
		wantErr        string
		wantKey        string
		wantDeprecated bool
	}{
		{name: "unset", env: map[string]string{}},
		{name: "new name only", env: map[string]string{"ENCRYPTION_KEY": key}, wantKey: key},
		{
			name:           "deprecated name only",
			env:            map[string]string{"EMAIL_ENCRYPTION_KEY": key},
			wantKey:        key,
			wantDeprecated: true,
		},
		{
			name:           "both equal",
			env:            map[string]string{"ENCRYPTION_KEY": key, "EMAIL_ENCRYPTION_KEY": key},
			wantKey:        key,
			wantDeprecated: true,
		},
		{
			name:    "both different",
			env:     map[string]string{"ENCRYPTION_KEY": key, "EMAIL_ENCRYPTION_KEY": other},
			wantErr: "different values",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("JWT_SECRET", "test")
			for _, k := range []string{"ENCRYPTION_KEY", "EMAIL_ENCRYPTION_KEY"} {
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
				t.Fatalf("Load() error = %v", err)
			}
			if cfg.EncryptionKey != tt.wantKey {
				t.Errorf("EncryptionKey = %q, want %q", cfg.EncryptionKey, tt.wantKey)
			}
			if cfg.UsesDeprecatedEncryptionKey() != tt.wantDeprecated {
				t.Errorf("UsesDeprecatedEncryptionKey() = %v, want %v", cfg.UsesDeprecatedEncryptionKey(), tt.wantDeprecated)
			}
		})
	}
}
