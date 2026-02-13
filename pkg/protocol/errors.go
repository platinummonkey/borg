package protocol

import "errors"

var (
	// ErrEmptyMessage is returned when parsing an empty or whitespace-only string.
	ErrEmptyMessage = errors.New("empty message")

	// ErrUnknownAction is returned when the action prefix is not a recognized protocol action.
	ErrUnknownAction = errors.New("unknown action")

	// ErrUnclosedQuote is returned when a quoted value is missing its closing quote.
	ErrUnclosedQuote = errors.New("unclosed quote in value")

	// ErrCredentialDetected is returned by the sanitizer when a message contains credential-like content.
	ErrCredentialDetected = errors.New("message contains credential-like content")
)
