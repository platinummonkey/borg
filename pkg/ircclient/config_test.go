package ircclient

import (
	"strings"
	"testing"
	"time"
)

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()

	if !cfg.TLS {
		t.Error("default config should have TLS enabled")
	}
	if !cfg.SASL {
		t.Error("default config should have SASL enabled")
	}
	if cfg.SASLMech != "PLAIN" {
		t.Errorf("default SASL mechanism should be PLAIN, got %q", cfg.SASLMech)
	}
	if !cfg.Reconnect {
		t.Error("default config should have reconnect enabled")
	}
	if cfg.MaxReconnectAttempts != 3 {
		t.Errorf("default max reconnect attempts should be 3, got %d", cfg.MaxReconnectAttempts)
	}
	if cfg.RateLimit != 2.0 {
		t.Errorf("default rate limit should be 2.0, got %f", cfg.RateLimit)
	}
	if cfg.RateLimitBurst != 5 {
		t.Errorf("default rate limit burst should be 5, got %d", cfg.RateLimitBurst)
	}
	if cfg.ReconnectBackoff != 1*time.Second {
		t.Errorf("default reconnect backoff should be 1s, got %v", cfg.ReconnectBackoff)
	}
	if cfg.MaxReconnectBackoff != 2*time.Minute {
		t.Errorf("default max reconnect backoff should be 2m, got %v", cfg.MaxReconnectBackoff)
	}
}

func TestValidate_Valid(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Server = "localhost:6697"
	cfg.Nick = "testnick"
	cfg.Username = "testuser"
	cfg.Password = "testpass"

	if err := cfg.Validate(); err != nil {
		t.Errorf("expected no error, got: %v", err)
	}
}

func TestValidate_MissingServer(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Nick = "testnick"
	cfg.Username = "testuser"
	cfg.Password = "testpass"

	err := cfg.Validate()
	if err == nil {
		t.Fatal("expected error for missing server")
	}
	if !strings.Contains(err.Error(), "server address is required") {
		t.Errorf("error should mention server, got: %v", err)
	}
}

func TestValidate_MissingNick(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Server = "localhost:6697"
	cfg.Username = "testuser"
	cfg.Password = "testpass"

	err := cfg.Validate()
	if err == nil {
		t.Fatal("expected error for missing nick")
	}
	if !strings.Contains(err.Error(), "nick is required") {
		t.Errorf("error should mention nick, got: %v", err)
	}
}

func TestValidate_MissingCredentials(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Server = "localhost:6697"
	cfg.Nick = "testnick"

	err := cfg.Validate()
	if err == nil {
		t.Fatal("expected error for missing credentials")
	}
	if !strings.Contains(err.Error(), "username is required") {
		t.Errorf("error should mention username, got: %v", err)
	}
	if !strings.Contains(err.Error(), "password is required") {
		t.Errorf("error should mention password, got: %v", err)
	}
}

func TestValidate_TLSDisabled(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Server = "localhost:6697"
	cfg.Nick = "testnick"
	cfg.Username = "testuser"
	cfg.Password = "testpass"
	cfg.TLS = false

	err := cfg.Validate()
	if err == nil {
		t.Fatal("expected error for disabled TLS")
	}
	if !strings.Contains(err.Error(), "TLS is required") {
		t.Errorf("error should mention TLS, got: %v", err)
	}
}

func TestValidate_SASLDisabled(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Server = "localhost:6697"
	cfg.Nick = "testnick"
	cfg.Username = "testuser"
	cfg.Password = "testpass"
	cfg.SASL = false

	err := cfg.Validate()
	if err == nil {
		t.Fatal("expected error for disabled SASL")
	}
	if !strings.Contains(err.Error(), "SASL authentication is required") {
		t.Errorf("error should mention SASL, got: %v", err)
	}
}

func TestValidate_MultipleErrors(t *testing.T) {
	cfg := Config{} // everything missing/false

	err := cfg.Validate()
	if err == nil {
		t.Fatal("expected error for empty config")
	}
	errStr := err.Error()
	// Should contain multiple validation errors.
	for _, expected := range []string{"server", "nick", "username", "password", "TLS", "SASL"} {
		if !strings.Contains(strings.ToLower(errStr), strings.ToLower(expected)) {
			t.Errorf("error should mention %q, got: %v", expected, errStr)
		}
	}
}
