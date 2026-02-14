// Package protocol defines the structured message format used for
// inter-agent communication over IRC.
//
// Messages follow the format:
//
//	[ACTION] key=value key2=value2 #tag1 #tag2
//
// The package provides parsing ([Parse]), serialization ([Message.String]),
// and input sanitization ([Sanitize]) to prevent protocol injection.
package protocol
