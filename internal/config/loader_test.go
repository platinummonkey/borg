package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoad_Defaults(t *testing.T) {
	cfg, err := Load([]string{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !cfg.IRC.TLS {
		t.Error("default TLS should be true")
	}
	if !cfg.IRC.SASL {
		t.Error("default SASL should be true")
	}
	if cfg.LogLevel != "info" {
		t.Errorf("default log level should be info, got %q", cfg.LogLevel)
	}
}

func TestLoad_CLIFlags(t *testing.T) {
	args := []string{
		"--server", "irc.example.com:6697",
		"--nick", "testagent",
		"--username", "testuser",
		"--password", "testpass",
		"--channels", "#test,#dev",
		"--log-level", "debug",
	}

	cfg, err := Load(args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.IRC.Server != "irc.example.com:6697" {
		t.Errorf("server = %q, want irc.example.com:6697", cfg.IRC.Server)
	}
	if cfg.IRC.Nick != "testagent" {
		t.Errorf("nick = %q, want testagent", cfg.IRC.Nick)
	}
	if cfg.IRC.Username != "testuser" {
		t.Errorf("username = %q, want testuser", cfg.IRC.Username)
	}
	if cfg.IRC.Password != "testpass" {
		t.Errorf("password = %q, want testpass", cfg.IRC.Password)
	}
	if len(cfg.IRC.Channels) != 2 || cfg.IRC.Channels[0] != "#test" || cfg.IRC.Channels[1] != "#dev" {
		t.Errorf("channels = %v, want [#test, #dev]", cfg.IRC.Channels)
	}
	if cfg.LogLevel != "debug" {
		t.Errorf("log level = %q, want debug", cfg.LogLevel)
	}
}

func TestLoad_EnvVars(t *testing.T) {
	t.Setenv("IRC_SERVER", "env.example.com:6697")
	t.Setenv("IRC_NICK", "envagent")
	t.Setenv("IRC_USERNAME", "envuser")
	t.Setenv("IRC_PASSWORD", "envpass")
	t.Setenv("IRC_CHANNELS", "#env1, #env2")
	t.Setenv("LOG_LEVEL", "warn")

	cfg, err := Load([]string{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.IRC.Server != "env.example.com:6697" {
		t.Errorf("server = %q, want env.example.com:6697", cfg.IRC.Server)
	}
	if cfg.IRC.Nick != "envagent" {
		t.Errorf("nick = %q, want envagent", cfg.IRC.Nick)
	}
	if len(cfg.IRC.Channels) != 2 {
		t.Errorf("channels = %v, want 2 channels", cfg.IRC.Channels)
	}
	if cfg.LogLevel != "warn" {
		t.Errorf("log level = %q, want warn", cfg.LogLevel)
	}
}

func TestLoad_CLIFlagsOverrideEnv(t *testing.T) {
	t.Setenv("IRC_SERVER", "env.example.com:6697")

	cfg, err := Load([]string{"--server", "cli.example.com:6697"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.IRC.Server != "cli.example.com:6697" {
		t.Errorf("server = %q, want cli.example.com:6697 (CLI should override env)", cfg.IRC.Server)
	}
}

func TestLoad_ConfigFile(t *testing.T) {
	yamlContent := `
irc:
  server: "file.example.com:6697"
  nick: "fileagent"
  username: "fileuser"
  password: "filepass"
  channels:
    - "#file1"
  tls: true
  sasl: true
log_level: "error"
`
	tmpDir := t.TempDir()
	cfgPath := filepath.Join(tmpDir, "config.yaml")
	if err := os.WriteFile(cfgPath, []byte(yamlContent), 0644); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load([]string{"--config", cfgPath})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.IRC.Server != "file.example.com:6697" {
		t.Errorf("server = %q, want file.example.com:6697", cfg.IRC.Server)
	}
	if cfg.IRC.Nick != "fileagent" {
		t.Errorf("nick = %q, want fileagent", cfg.IRC.Nick)
	}
	if cfg.LogLevel != "error" {
		t.Errorf("log level = %q, want error", cfg.LogLevel)
	}
}

func TestLoad_PriorityOrder(t *testing.T) {
	// File sets server to "file", env sets it to "env", CLI sets it to "cli".
	yamlContent := `
irc:
  server: "file:6697"
  nick: "filenick"
  username: "fileuser"
  password: "filepass"
  tls: true
  sasl: true
`
	tmpDir := t.TempDir()
	cfgPath := filepath.Join(tmpDir, "config.yaml")
	if err := os.WriteFile(cfgPath, []byte(yamlContent), 0644); err != nil {
		t.Fatal(err)
	}

	t.Setenv("IRC_SERVER", "env:6697")
	t.Setenv("IRC_NICK", "envnick")

	cfg, err := Load([]string{"--config", cfgPath, "--server", "cli:6697"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// CLI > env > file: server should be "cli:6697"
	if cfg.IRC.Server != "cli:6697" {
		t.Errorf("server = %q, want cli:6697", cfg.IRC.Server)
	}
	// env > file for nick (no CLI flag set for nick)
	if cfg.IRC.Nick != "envnick" {
		t.Errorf("nick = %q, want envnick (env overrides file)", cfg.IRC.Nick)
	}
}

func TestSplitChannels(t *testing.T) {
	tests := []struct {
		input string
		want  []string
	}{
		{"", nil},
		{"#test", []string{"#test"}},
		{"#test, #dev", []string{"#test", "#dev"}},
		{"#test,#dev,#ops", []string{"#test", "#dev", "#ops"}},
		{" #test , #dev ", []string{"#test", "#dev"}},
	}

	for _, tt := range tests {
		got := splitChannels(tt.input)
		if len(got) != len(tt.want) {
			t.Errorf("splitChannels(%q) = %v, want %v", tt.input, got, tt.want)
			continue
		}
		for i := range got {
			if strings.TrimSpace(got[i]) != strings.TrimSpace(tt.want[i]) {
				t.Errorf("splitChannels(%q)[%d] = %q, want %q", tt.input, i, got[i], tt.want[i])
			}
		}
	}
}
