package protocol

import (
	"fmt"
	"regexp"
	"strings"
)

// sensitiveKeys are field key names that suggest credential content.
var sensitiveKeys = map[string]struct{}{
	"password":    {},
	"passwd":      {},
	"secret":      {},
	"token":       {},
	"api_key":     {},
	"apikey":      {},
	"api-key":     {},
	"auth":        {},
	"credential":  {},
	"credentials": {},
	"private_key": {},
	"private-key": {},
	"access_key":  {},
	"access-key":  {},
	"secret_key":  {},
	"secret-key":  {},
}

// credentialPatterns matches common credential-like patterns in values.
var credentialPatterns = []*regexp.Regexp{
	// AWS-style keys: AKIA followed by 16 alphanumeric chars.
	regexp.MustCompile(`(?i)AKIA[0-9A-Z]{16}`),
	// Bearer tokens.
	regexp.MustCompile(`(?i)bearer\s+[a-zA-Z0-9\-_.]+`),
	// Generic long hex strings that look like secrets (40+ hex chars).
	regexp.MustCompile(`[0-9a-fA-F]{40,}`),
	// GitHub tokens.
	regexp.MustCompile(`gh[ps]_[A-Za-z0-9_]{36,}`),
	// Slack tokens.
	regexp.MustCompile(`xox[bpras]-[0-9A-Za-z\-]+`),
}

// Sanitize checks a Message for credential-like content and returns
// ErrCredentialDetected if any is found. Call this before sending.
func Sanitize(msg *Message) error {
	// Check field keys.
	for k := range msg.Fields {
		if _, ok := sensitiveKeys[strings.ToLower(k)]; ok {
			return fmt.Errorf("%w: sensitive key %q", ErrCredentialDetected, k)
		}
	}

	// Check field values for credential patterns.
	for k, v := range msg.Fields {
		for _, pat := range credentialPatterns {
			if pat.MatchString(v) {
				return fmt.Errorf("%w: credential pattern in field %q", ErrCredentialDetected, k)
			}
		}
	}

	// Check payload for credential patterns.
	if msg.Payload != "" {
		for _, pat := range credentialPatterns {
			if pat.MatchString(msg.Payload) {
				return fmt.Errorf("%w: credential pattern in payload", ErrCredentialDetected)
			}
		}
	}

	return nil
}
