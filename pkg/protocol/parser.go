package protocol

import (
	"fmt"
	"strings"
)

// Parse parses a raw protocol message string into a Message.
// The expected wire format is:
//
//	ACTION key=value key2="quoted value" #tag1 #tag2
//
// For SHARING-CONTEXT, everything after the action is treated as raw payload.
func Parse(raw string) (*Message, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, ErrEmptyMessage
	}

	// Extract the action (first whitespace-delimited token).
	actionStr, rest := splitFirst(raw)
	action := Action(strings.ToUpper(actionStr))
	if !action.Valid() {
		return nil, fmt.Errorf("%w: %s", ErrUnknownAction, actionStr)
	}

	msg := &Message{
		Action: action,
		Fields: make(map[string]string),
	}

	// SHARING-CONTEXT treats the rest as raw payload.
	if action == ActionSharingContext {
		msg.Payload = rest
		return msg, nil
	}

	// Tokenize the remainder into fields and tags.
	if err := tokenize(msg, rest); err != nil {
		return nil, err
	}

	return msg, nil
}

// splitFirst splits s into the first whitespace-delimited token and the rest.
func splitFirst(s string) (first, rest string) {
	i := strings.IndexAny(s, " \t")
	if i < 0 {
		return s, ""
	}
	return s[:i], strings.TrimSpace(s[i+1:])
}

// tokenize parses the body of a protocol message (after the action) into
// key=value fields and #tags on the message.
func tokenize(msg *Message, body string) error {
	i := 0
	for i < len(body) {
		// Skip whitespace.
		if body[i] == ' ' || body[i] == '\t' {
			i++
			continue
		}

		// Tag: starts with #
		if body[i] == '#' {
			i++ // skip #
			start := i
			for i < len(body) && body[i] != ' ' && body[i] != '\t' {
				i++
			}
			if i > start {
				msg.Tags = append(msg.Tags, body[start:i])
			}
			continue
		}

		// Key=value or bare word.
		start := i
		for i < len(body) && body[i] != '=' && body[i] != ' ' && body[i] != '\t' {
			i++
		}

		if i >= len(body) || body[i] != '=' {
			// Bare word — skip it (not a valid field or tag).
			continue
		}

		key := body[start:i]
		i++ // skip '='

		// Parse value: quoted or unquoted.
		if i < len(body) && body[i] == '"' {
			i++ // skip opening quote
			start = i
			for i < len(body) && body[i] != '"' {
				i++
			}
			if i >= len(body) {
				return fmt.Errorf("%w: key=%s", ErrUnclosedQuote, key)
			}
			msg.Fields[key] = body[start:i]
			i++ // skip closing quote
		} else {
			start = i
			for i < len(body) && body[i] != ' ' && body[i] != '\t' {
				i++
			}
			msg.Fields[key] = body[start:i]
		}
	}
	return nil
}
