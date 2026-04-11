package config

import (
	"flag"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"gopkg.in/yaml.v3"

	agentOtel "github.com/platinummonkey/borg/internal/otel"
	"github.com/platinummonkey/borg/pkg/ircclient"
	"github.com/platinummonkey/borg/pkg/protocol"
)

// ACLRule defines an authorization rule for protocol messages.
type ACLRule struct {
	Channel     string            `yaml:"channel"`
	NickPattern string            `yaml:"nick_pattern"`
	Actions     []protocol.Action `yaml:"actions"`
	Effect      string            `yaml:"effect"`
}

// FederationServerConfig describes a remote IRC server for federation.
type FederationServerConfig struct {
	Name string           `yaml:"name"`
	IRC  ircclient.Config `yaml:"irc"`
}

// ChannelMapping maps a local channel to a remote channel on a specific link.
type ChannelMapping struct {
	LocalChannel  string `yaml:"local_channel"`
	RemoteChannel string `yaml:"remote_channel"`
	LinkName      string `yaml:"link_name"`
}

// AppConfig combines IRC client config with application-level settings.
type AppConfig struct {
	IRC                ircclient.Config         `yaml:"irc"`
	LogLevel           string                   `yaml:"log_level"`
	LogFmt             string                   `yaml:"log_format"`
	DashboardAddr      string                   `yaml:"dashboard_addr"`
	StateFile          string                   `yaml:"state_file"`
	ACLRules           []ACLRule                `yaml:"acl_rules"`
	Capabilities       []string                 `yaml:"capabilities"`
	DiscoveryTTL       time.Duration            `yaml:"discovery_ttl"`
	FederationServers  []FederationServerConfig  `yaml:"federation_servers"`
	FederationMappings []ChannelMapping          `yaml:"federation_mappings"`
	OTel               agentOtel.Config          `yaml:"otel"`

	// Coordination (Phases 12–16).
	AgentRole          string        `yaml:"agent_role"`
	Roles              []string      `yaml:"roles"`
	MaxLoad            int           `yaml:"max_load"`
	ClaimJitter        time.Duration `yaml:"claim_jitter"`
	MaxReviewIterations int          `yaml:"max_review_iterations"`
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
		ReconnectBackoff      string   `yaml:"reconnect_backoff"`
		MaxReconnectBackoff   string   `yaml:"max_reconnect_backoff"`
		RateLimit             *float64 `yaml:"rate_limit"`
		RateLimitBurst        *int     `yaml:"rate_limit_burst"`
		PingFrequency         string   `yaml:"ping_frequency"`
		Timeout               string   `yaml:"timeout"`
		QuitMessage           string   `yaml:"quit_message"`
		Debug                 bool     `yaml:"debug"`
	} `yaml:"irc"`
	LogLevel      string    `yaml:"log_level"`
	LogFmt        string    `yaml:"log_format"`
	DashboardAddr string    `yaml:"dashboard_addr"`
	StateFile     string    `yaml:"state_file"`
	ACLRules      []ACLRule `yaml:"acl_rules"`
	Capabilities       []string                  `yaml:"capabilities"`
	DiscoveryTTL       string                    `yaml:"discovery_ttl"`
	FederationServers  []FederationServerConfig  `yaml:"federation_servers"`
	FederationMappings []ChannelMapping          `yaml:"federation_mappings"`
	OTel               struct {
		Endpoint    string  `yaml:"endpoint"`
		ServiceName string  `yaml:"service_name"`
		SampleRate  float64 `yaml:"sample_rate"`
	} `yaml:"otel"`
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
	fs := flag.NewFlagSet("borg", flag.ContinueOnError)
	fs.Usage = func() {
		_, _ = fmt.Fprintf(fs.Output(), "Usage: borg [flags]\n\nFlags:\n")
		fs.PrintDefaults()
	}
	var (
		flagServer        = fs.String("server", "", "IRC server address (host:port)")
		flagNick          = fs.String("nick", "", "Agent nickname")
		flagUsername      = fs.String("username", "", "IRC/SASL username")
		flagPassword      = fs.String("password", "", "SASL password")
		flagRealName      = fs.String("realname", "", "IRC real name")
		flagChannels      = fs.String("channels", "", "Comma-separated channels to join")
		flagTLS           = fs.Bool("tls", true, "Enable TLS (required)")
		flagTLSInsecure   = fs.Bool("tls-insecure", false, "Skip TLS certificate verification")
		flagSASL          = fs.Bool("sasl", true, "Enable SASL (required)")
		flagConfigFile    = fs.String("config", "", "Path to YAML config file")
		flagLogLevel      = fs.String("log-level", "", "Log level (debug, info, warn, error)")
		flagLogFmt        = fs.String("log-format", "", "Log format (text, json)")
		flagDebug         = fs.Bool("debug", false, "Enable IRC debug logging")
		flagDashboardAddr = fs.String("dashboard-addr", "", "HTTP dashboard listen address (e.g. :8080)")
		flagRateLimit     = fs.Float64("rate-limit", 0, "Max outgoing messages per second (0 = use default)")
		flagRateBurst     = fs.Int("rate-limit-burst", 0, "Max burst for outgoing messages (0 = use default)")
		flagStateFile     = fs.String("state-file", "", "Path to persist task/dependency state (empty = disabled)")
		flagCapabilities   = fs.String("capabilities", "", "Comma-separated agent expertise tags for discovery")
		flagOTelEndpoint   = fs.String("otel-endpoint", "", "OTLP HTTP endpoint (empty = disabled)")
		flagOTelService    = fs.String("otel-service-name", "", "OTel service name (default: borg)")

		flagRole              = fs.String("role", "", "Agent role (e.g. implementer, reviewer)")
		flagRoles             = fs.String("roles", "", "Comma-separated agent roles")
		flagMaxLoad           = fs.Int("max-load", 0, "Maximum concurrent task load for this agent")
		flagClaimJitter       = fs.String("claim-jitter", "", "Claim arbitration window duration (e.g. 2s)")
		flagMaxReviewIter     = fs.Int("max-review-iterations", 0, "Max review iterations before auto-escalation (0 = unlimited)")
	)

	if err := fs.Parse(args); err != nil {
		return nil, err
	}

	// Layer 2: Config file (overrides defaults).
	configPath := *flagConfigFile
	if configPath == "" {
		configPath = os.Getenv("BORG_CONFIG")
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
		case "dashboard-addr":
			cfg.DashboardAddr = *flagDashboardAddr
		case "rate-limit":
			cfg.IRC.RateLimit = *flagRateLimit
		case "rate-limit-burst":
			cfg.IRC.RateLimitBurst = *flagRateBurst
		case "state-file":
			cfg.StateFile = *flagStateFile
		case "capabilities":
			cfg.Capabilities = splitChannels(*flagCapabilities) // reuse comma splitter
		case "otel-endpoint":
			cfg.OTel.Endpoint = *flagOTelEndpoint
		case "otel-service-name":
			cfg.OTel.ServiceName = *flagOTelService
		case "role":
			cfg.AgentRole = *flagRole
		case "roles":
			cfg.Roles = splitChannels(*flagRoles)
		case "max-load":
			cfg.MaxLoad = *flagMaxLoad
		case "claim-jitter":
			if d, err := time.ParseDuration(*flagClaimJitter); err == nil {
				cfg.ClaimJitter = d
			}
		case "max-review-iterations":
			cfg.MaxReviewIterations = *flagMaxReviewIter
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
	if fc.IRC.ReconnectBackoff != "" {
		if d, err := time.ParseDuration(fc.IRC.ReconnectBackoff); err == nil {
			cfg.IRC.ReconnectBackoff = d
		}
	}
	if fc.IRC.MaxReconnectBackoff != "" {
		if d, err := time.ParseDuration(fc.IRC.MaxReconnectBackoff); err == nil {
			cfg.IRC.MaxReconnectBackoff = d
		}
	}
	if fc.IRC.RateLimit != nil {
		cfg.IRC.RateLimit = *fc.IRC.RateLimit
	}
	if fc.IRC.RateLimitBurst != nil {
		cfg.IRC.RateLimitBurst = *fc.IRC.RateLimitBurst
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
	if fc.DashboardAddr != "" {
		cfg.DashboardAddr = fc.DashboardAddr
	}
	if fc.StateFile != "" {
		cfg.StateFile = fc.StateFile
	}
	if len(fc.ACLRules) > 0 {
		cfg.ACLRules = fc.ACLRules
	}
	if len(fc.Capabilities) > 0 {
		cfg.Capabilities = fc.Capabilities
	}
	if fc.DiscoveryTTL != "" {
		if d, err := time.ParseDuration(fc.DiscoveryTTL); err == nil {
			cfg.DiscoveryTTL = d
		}
	}
	if len(fc.FederationServers) > 0 {
		cfg.FederationServers = fc.FederationServers
	}
	if len(fc.FederationMappings) > 0 {
		cfg.FederationMappings = fc.FederationMappings
	}
	if fc.OTel.Endpoint != "" {
		cfg.OTel.Endpoint = fc.OTel.Endpoint
	}
	if fc.OTel.ServiceName != "" {
		cfg.OTel.ServiceName = fc.OTel.ServiceName
	}
	if fc.OTel.SampleRate > 0 {
		cfg.OTel.SampleRate = fc.OTel.SampleRate
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
	if v := os.Getenv("DASHBOARD_ADDR"); v != "" {
		cfg.DashboardAddr = v
	}
	if v := os.Getenv("STATE_FILE"); v != "" {
		cfg.StateFile = v
	}
	if v := os.Getenv("AGENT_CAPABILITIES"); v != "" {
		cfg.Capabilities = splitChannels(v)
	}
	if v := os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT"); v != "" {
		cfg.OTel.Endpoint = v
	}
	if v := os.Getenv("AGENT_ROLE"); v != "" {
		cfg.AgentRole = v
	}
	if v := os.Getenv("AGENT_ROLES"); v != "" {
		cfg.Roles = splitChannels(v)
	}
	if v := os.Getenv("MAX_LOAD"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			cfg.MaxLoad = n
		}
	}
	if v := os.Getenv("CLAIM_JITTER"); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			cfg.ClaimJitter = d
		}
	}
	if v := os.Getenv("MAX_REVIEW_ITERATIONS"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			cfg.MaxReviewIterations = n
		}
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
