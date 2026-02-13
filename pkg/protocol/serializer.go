package protocol

import (
	"sort"
	"strings"
)

// String serializes a Message back to the wire format.
// Fields are sorted by key for deterministic output.
// Values containing spaces are automatically quoted.
func (m *Message) String() string {
	var b strings.Builder

	b.WriteString(string(m.Action))

	// SHARING-CONTEXT: append raw payload.
	if m.Action == ActionSharingContext {
		if m.Payload != "" {
			b.WriteByte(' ')
			b.WriteString(m.Payload)
		}
		return b.String()
	}

	// Sort keys for deterministic output.
	keys := make([]string, 0, len(m.Fields))
	for k := range m.Fields {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	for _, k := range keys {
		v := m.Fields[k]
		b.WriteByte(' ')
		b.WriteString(k)
		b.WriteByte('=')
		if strings.ContainsAny(v, " \t") {
			b.WriteByte('"')
			b.WriteString(v)
			b.WriteByte('"')
		} else {
			b.WriteString(v)
		}
	}

	for _, tag := range m.Tags {
		b.WriteByte(' ')
		b.WriteByte('#')
		b.WriteString(tag)
	}

	return b.String()
}
