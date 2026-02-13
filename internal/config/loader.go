package config

import (
	"flag"
	"os"
	"strings"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/platinummonkey/agent-chat/pkg/ircclient"
)

// AppConfig combines IRC client config with application-level settings.
type AppConfig struct {
	IRC      ircclient.Config `yaml:"irc"`
	LogLevel string           `yaml:"log_level"`
	LogFmt   string           `yaml:"log_format"`
}

// fileConfig mirrors AppConfig for YAML unmarshaling with duration strings.
type fileConfig struct {
	IRC struct {
		Server                string   `yaml:"server"`
		Nick                  string   `yaml:"nick"`
		Username              string   `yaml:"username"`
		Password              string   `yaml:"password"`
		RealName              string   `yaml:"realname"`
		TLS                   *bool    `yaml:"tls"`
		TLSInsecureSkipVerify bool     `yaml:"tls_insecure_skip_verify"`
		SASL                  *bool    `yaml:"sasl"`
		SASLMech              string   `yaml:"sasl_mech"`
		Channels              []string `yaml:"channels"`
		AutoRejoinOnKick      bool     `yaml:"auto_rejoin_on_kick"`
		Reconnect             *bool    `yaml:"reconnect"`
		MaxReconnectAttempts  int      `yaml:"max_reconnect_attempts"`
		PingFrequency         string   `yaml:"ping_frequency"`
		Timeout               string   `yaml:"timeout"`
		QuitMessage           string   `yaml:"quit_message"`
		Debug                 bool     `yaml:"debug"`
	} `yaml:"irc"`
	LogLevel string `yaml:"log_level"`
	LogFmt   string `yaml:"log_format"`
}

// Load builds an AppConfig from defaults, config file, environment variables, and CLI flags.
// Priority: flags > env > file > defaults.
func Load(args []string) (*AppConfig, error) {
	cfg := &AppConfig{
		IRC:      ircclient.DefaultConfig(),
		LogLevel: "info",
		LogFmt:   "text",
	}

	// Parse flags (but don't apply yet — we need to know what was explicitly set).
	fs := flag.NewFlagSet("agent-chat", flag.ContinueOnError)
	var (
		flagServer      = fs.String("server", "", "IRC server address (host:port)")
		flagNick        = fs.String("nick", "", "Agent nickname")
		flagUsername    = fs.String("username", "", "IRC/SASL username")
		flagPassword    = fs.String("password", "", "SASL password")
		flagRealName    = fs.String("realname", "", "IRC real name")
		flagChannels    = fs.String("channels", "", "Comma-separated channels to join")
		flagTLS         = fs.Bool("tls", true, "Enable TLS (required)")
		flagTLSInsecure = fs.Bool("tls-insecure", false, "Skip TLS certificate verification")
		flagSASL        = fs.Bool("sasl", true, "Enable SASL (required)")
		flagConfigFile  = fs.String("config", "", "Path to YAML config file")
		flagLogLevel    = fs.String("log-level", "", "Log level (debug, info, warn, error)")
		flagLogFmt      = fs.String("log-format", "", "Log format (text, json)")
		flagDebug       = fs.Bool("debug", false, "Enable IRC debug logging")
	)

	if err := fs.Parse(args); err != nil {
		return nil, err
	}

	// Layer 2: Config file (overrides defaults).
	configPath := *flagConfigFile
	if configPath == "" {
		configPath = os.Getenv("AGENT_CHAT_CONFIG")
	}
	if configPath != "" {
		if err := loadFromFile(cfg, configPath); err != nil {
			return nil, err
		}
	}

	// Layer 3: Environment variables (override file).
	applyEnv(cfg)

	// Layer 4: CLI flags (override everything).
	// Only apply flags that were explicitly set.
	fs.Visit(func(f *flag.Flag) {
		switch f.Name {
		case "server":
			cfg.IRC.Server = *flagServer
		case "nick":
			cfg.IRC.Nick = *flagNick
		case "username":
			cfg.IRC.Username = *flagUsername
		case "password":
			cfg.IRC.Password = *flagPassword
		case "realname":
			cfg.IRC.RealName = *flagRealName
		case "channels":
			cfg.IRC.Channels = splitChannels(*flagChannels)
		case "tls":
			cfg.IRC.TLS = *flagTLS
		case "tls-insecure":
			cfg.IRC.TLSInsecureSkipVerify = *flagTLSInsecure
		case "sasl":
			cfg.IRC.SASL = *flagSASL
		case "log-level":
			cfg.LogLevel = *flagLogLevel
		case "log-format":
			cfg.LogFmt = *flagLogFmt
		case "debug":
			cfg.IRC.Debug = *flagDebug
		}
	})

	return cfg, nil
}

func loadFromFile(cfg *AppConfig, path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}

	var fc fileConfig
	if err := yaml.Unmarshal(data, &fc); err != nil {
		return err
	}

	// Apply file values where set.
	if fc.IRC.Server != "" {
		cfg.IRC.Server = fc.IRC.Server
	}
	if fc.IRC.Nick != "" {
		cfg.IRC.Nick = fc.IRC.Nick
	}
	if fc.IRC.Username != "" {
		cfg.IRC.Username = fc.IRC.Username
	}
	if fc.IRC.Password != "" {
		cfg.IRC.Password = fc.IRC.Password
	}
	if fc.IRC.RealName != "" {
		cfg.IRC.RealName = fc.IRC.RealName
	}
	if fc.IRC.TLS != nil {
		cfg.IRC.TLS = *fc.IRC.TLS
	}
	cfg.IRC.TLSInsecureSkipVerify = fc.IRC.TLSInsecureSkipVerify
	if fc.IRC.SASL != nil {
		cfg.IRC.SASL = *fc.IRC.SASL
	}
	if fc.IRC.SASLMech != "" {
		cfg.IRC.SASLMech = fc.IRC.SASLMech
	}
	if len(fc.IRC.Channels) > 0 {
		cfg.IRC.Channels = fc.IRC.Channels
	}
	cfg.IRC.AutoRejoinOnKick = fc.IRC.AutoRejoinOnKick
	if fc.IRC.Reconnect != nil {
		cfg.IRC.Reconnect = *fc.IRC.Reconnect
	}
	if fc.IRC.MaxReconnectAttempts > 0 {
		cfg.IRC.MaxReconnectAttempts = fc.IRC.MaxReconnectAttempts
	}
	if fc.IRC.PingFrequency != "" {
		if d, err := time.ParseDuration(fc.IRC.PingFrequency); err == nil {
			cfg.IRC.PingFrequency = d
		}
	}
	if fc.IRC.Timeout != "" {
		if d, err := time.ParseDuration(fc.IRC.Timeout); err == nil {
			cfg.IRC.Timeout = d
		}
	}
	if fc.IRC.QuitMessage != "" {
		cfg.IRC.QuitMessage = fc.IRC.QuitMessage
	}
	cfg.IRC.Debug = fc.IRC.Debug

	if fc.LogLevel != "" {
		cfg.LogLevel = fc.LogLevel
	}
	if fc.LogFmt != "" {
		cfg.LogFmt = fc.LogFmt
	}

	return nil
}

func applyEnv(cfg *AppConfig) {
	if v := os.Getenv("IRC_SERVER"); v != "" {
		cfg.IRC.Server = v
	}
	if v := os.Getenv("IRC_NICK"); v != "" {
		cfg.IRC.Nick = v
	}
	if v := os.Getenv("IRC_USERNAME"); v != "" {
		cfg.IRC.Username = v
	}
	if v := os.Getenv("IRC_PASSWORD"); v != "" {
		cfg.IRC.Password = v
	}
	if v := os.Getenv("IRC_REALNAME"); v != "" {
		cfg.IRC.RealName = v
	}
	if v := os.Getenv("IRC_CHANNELS"); v != "" {
		cfg.IRC.Channels = splitChannels(v)
	}
	if v := os.Getenv("LOG_LEVEL"); v != "" {
		cfg.LogLevel = v
	}
	if v := os.Getenv("LOG_FORMAT"); v != "" {
		cfg.LogFmt = v
	}
}

func splitChannels(s string) []string {
	if s == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	channels := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			channels = append(channels, p)
		}
	}
	return channels
}
