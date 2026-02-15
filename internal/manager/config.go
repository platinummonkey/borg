package manager

import (
	"flag"
	"fmt"
	"os"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/platinummonkey/borg/pkg/ircclient"
)

// SSHDefaults holds default SSH connection parameters.
type SSHDefaults struct {
	User    string `yaml:"user"`
	KeyPath string `yaml:"key_path"`
}

// DockerDefaults holds default Docker parameters.
type DockerDefaults struct {
	Image   string `yaml:"image"`
	Network string `yaml:"network"`
}

// ManagerConfig holds all configuration for the manager process.
type ManagerConfig struct {
	IRC            ircclient.Config `yaml:"irc"`
	ListenAddr     string           `yaml:"listen_addr"`
	LogLevel       string           `yaml:"log_level"`
	LogFmt         string           `yaml:"log_format"`
	AgentBinary    string           `yaml:"agent_binary"`
	PollInterval   time.Duration    `yaml:"poll_interval"`
	SSHDefaults    SSHDefaults      `yaml:"ssh_defaults"`
	DockerDefaults DockerDefaults   `yaml:"docker_defaults"`
}

// fileConfig mirrors ManagerConfig for YAML with string durations.
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
		Debug                 bool     `yaml:"debug"`
	} `yaml:"irc"`
	ListenAddr     string         `yaml:"listen_addr"`
	LogLevel       string         `yaml:"log_level"`
	LogFmt         string         `yaml:"log_format"`
	AgentBinary    string         `yaml:"agent_binary"`
	PollInterval   string         `yaml:"poll_interval"`
	SSHDefaults    SSHDefaults    `yaml:"ssh_defaults"`
	DockerDefaults DockerDefaults `yaml:"docker_defaults"`
}

// LoadConfig builds a ManagerConfig from defaults, config file, env, and CLI flags.
func LoadConfig(args []string) (*ManagerConfig, error) {
	cfg := &ManagerConfig{
		IRC:          ircclient.DefaultConfig(),
		ListenAddr:   ":9090",
		LogLevel:     "info",
		LogFmt:       "text",
		PollInterval: 10 * time.Second,
	}

	fs := flag.NewFlagSet("queen", flag.ContinueOnError)
	fs.Usage = func() {
		fmt.Fprintf(fs.Output(), "Usage: queen [flags]\n\nFlags:\n")
		fs.PrintDefaults()
	}

	var (
		flagServer      = fs.String("server", "", "IRC server address (host:port)")
		flagNick        = fs.String("nick", "", "Manager nickname")
		flagUsername    = fs.String("username", "", "IRC/SASL username")
		flagPassword    = fs.String("password", "", "SASL password")
		flagChannels    = fs.String("channels", "", "Comma-separated channels to join")
		flagTLS         = fs.Bool("tls", true, "Enable TLS")
		flagTLSInsecure = fs.Bool("tls-insecure", false, "Skip TLS cert verification")
		flagSASL        = fs.Bool("sasl", true, "Enable SASL")
		flagListenAddr  = fs.String("listen-addr", "", "Web UI listen address")
		flagLogLevel    = fs.String("log-level", "", "Log level")
		flagLogFmt      = fs.String("log-format", "", "Log format")
		flagAgentBinary = fs.String("agent-binary", "", "Path to agent binary for local spawner")
		flagPollInterval = fs.String("poll-interval", "", "Dashboard poll interval (e.g. 10s)")
		flagConfigFile  = fs.String("config", "", "Path to YAML config file")
		flagDebug       = fs.Bool("debug", false, "Enable IRC debug logging")
	)

	if err := fs.Parse(args); err != nil {
		return nil, err
	}

	// Config file.
	configPath := *flagConfigFile
	if configPath == "" {
		configPath = os.Getenv("QUEEN_CONFIG")
	}
	if configPath != "" {
		if err := loadFromFile(cfg, configPath); err != nil {
			return nil, err
		}
	}

	// Environment variables.
	applyEnv(cfg)

	// CLI flags (highest priority).
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
		case "channels":
			cfg.IRC.Channels = splitComma(*flagChannels)
		case "tls":
			cfg.IRC.TLS = *flagTLS
		case "tls-insecure":
			cfg.IRC.TLSInsecureSkipVerify = *flagTLSInsecure
		case "sasl":
			cfg.IRC.SASL = *flagSASL
		case "listen-addr":
			cfg.ListenAddr = *flagListenAddr
		case "log-level":
			cfg.LogLevel = *flagLogLevel
		case "log-format":
			cfg.LogFmt = *flagLogFmt
		case "agent-binary":
			cfg.AgentBinary = *flagAgentBinary
		case "poll-interval":
			if d, err := time.ParseDuration(*flagPollInterval); err == nil {
				cfg.PollInterval = d
			}
		case "debug":
			cfg.IRC.Debug = *flagDebug
		}
	})

	return cfg, nil
}

func loadFromFile(cfg *ManagerConfig, path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	var fc fileConfig
	if err := yaml.Unmarshal(data, &fc); err != nil {
		return err
	}

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
	cfg.IRC.Debug = fc.IRC.Debug
	if fc.ListenAddr != "" {
		cfg.ListenAddr = fc.ListenAddr
	}
	if fc.LogLevel != "" {
		cfg.LogLevel = fc.LogLevel
	}
	if fc.LogFmt != "" {
		cfg.LogFmt = fc.LogFmt
	}
	if fc.AgentBinary != "" {
		cfg.AgentBinary = fc.AgentBinary
	}
	if fc.PollInterval != "" {
		if d, err := time.ParseDuration(fc.PollInterval); err == nil {
			cfg.PollInterval = d
		}
	}
	cfg.SSHDefaults = fc.SSHDefaults
	cfg.DockerDefaults = fc.DockerDefaults
	return nil
}

func applyEnv(cfg *ManagerConfig) {
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
	if v := os.Getenv("IRC_CHANNELS"); v != "" {
		cfg.IRC.Channels = splitComma(v)
	}
	if v := os.Getenv("MANAGER_LISTEN_ADDR"); v != "" {
		cfg.ListenAddr = v
	}
	if v := os.Getenv("MANAGER_AGENT_BINARY"); v != "" {
		cfg.AgentBinary = v
	}
	if v := os.Getenv("LOG_LEVEL"); v != "" {
		cfg.LogLevel = v
	}
	if v := os.Getenv("LOG_FORMAT"); v != "" {
		cfg.LogFmt = v
	}
}

func splitComma(s string) []string {
	if s == "" {
		return nil
	}
	var result []string
	for _, p := range splitBy(s, ',') {
		if p != "" {
			result = append(result, p)
		}
	}
	return result
}

func splitBy(s string, sep byte) []string {
	var result []string
	start := 0
	for i := range len(s) {
		if s[i] == sep {
			result = append(result, trimSpace(s[start:i]))
			start = i + 1
		}
	}
	result = append(result, trimSpace(s[start:]))
	return result
}

func trimSpace(s string) string {
	i, j := 0, len(s)
	for i < j && (s[i] == ' ' || s[i] == '\t') {
		i++
	}
	for j > i && (s[j-1] == ' ' || s[j-1] == '\t') {
		j--
	}
	return s[i:j]
}
