package ircclient

import (
	"errors"
	"fmt"
	"time"
)

// Config holds all configuration for an IRC client connection.
type Config struct {
	// Server is the IRC server address in host:port format.
	Server string `yaml:"server"`

	// Nick is the desired nickname for the agent.
	Nick string `yaml:"nick"`

	// Username is the IRC username (USER command).
	Username string `yaml:"username"`

	// Password is the SASL authentication password.
	Password string `yaml:"password"`

	// RealName is the IRC real name field.
	RealName string `yaml:"realname"`

	// TLS enables TLS/SSL for the connection. Must be true.
	TLS bool `yaml:"tls"`

	// TLSInsecureSkipVerify disables certificate verification (for self-signed certs in dev).
	TLSInsecureSkipVerify bool `yaml:"tls_insecure_skip_verify"`

	// SASL enables SASL authentication. Must be true.
	SASL bool `yaml:"sasl"`

	// SASLMech is the SASL mechanism (default: "PLAIN").
	SASLMech string `yaml:"sasl_mech"`

	// Channels is the list of channels to join on connect.
	Channels []string `yaml:"channels"`

	// AutoRejoinOnKick enables automatic channel rejoin after being kicked.
	AutoRejoinOnKick bool `yaml:"auto_rejoin_on_kick"`

	// Reconnect enables automatic reconnection on disconnect.
	Reconnect bool `yaml:"reconnect"`

	// MaxReconnectAttempts is the maximum number of reconnection attempts.
	MaxReconnectAttempts int `yaml:"max_reconnect_attempts"`

	// PingFrequency is how often to send PING to the server.
	PingFrequency time.Duration `yaml:"ping_frequency"`

	// Timeout is the read/write timeout for the connection.
	Timeout time.Duration `yaml:"timeout"`

	// QuitMessage is the message sent when disconnecting.
	QuitMessage string `yaml:"quit_message"`

	// Debug enables debug-level logging in the IRC library.
	Debug bool `yaml:"debug"`
}

// DefaultConfig returns a Config with secure defaults.
func DefaultConfig() Config {
	return Config{
		TLS:                  true,
		SASL:                 true,
		SASLMech:             "PLAIN",
		Reconnect:            true,
		MaxReconnectAttempts: 3,
		PingFrequency:        2 * time.Minute,
		Timeout:              60 * time.Second,
		QuitMessage:          "agent shutting down",
		AutoRejoinOnKick:     false,
	}
}

// Validate checks that the configuration is complete and secure.
func (c *Config) Validate() error {
	var errs []error

	if c.Server == "" {
		errs = append(errs, errors.New("server address is required"))
	}
	if c.Nick == "" {
		errs = append(errs, errors.New("nick is required"))
	}
	if c.Username == "" {
		errs = append(errs, errors.New("username is required"))
	}
	if c.Password == "" {
		errs = append(errs, errors.New("password is required"))
	}
	if !c.TLS {
		errs = append(errs, errors.New("TLS is required; insecure connections are not allowed"))
	}
	if !c.SASL {
		errs = append(errs, errors.New("SASL authentication is required; unauthenticated connections are not allowed"))
	}

	if len(errs) > 0 {
		return fmt.Errorf("config validation failed: %w", errors.Join(errs...))
	}
	return nil
}
