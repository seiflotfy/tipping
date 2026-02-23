package tipping

import (
	"fmt"
	"strings"
)

// CorpusCodec is an exact-message codec trained on a fixed corpus.
// It is intended for compression/decompression when encode-time messages
// are expected to be from the same corpus used at training time.
type CorpusCodec struct {
	templates []string
	entries   map[string]codecEntry
}

type codecEntry struct {
	templateID int
	args       []string
}

// NewCorpusCodec trains an exact-message codec from messages.
// If parser is nil, NewParser() is used.
func NewCorpusCodec(messages []string, parser *Parser) (*CorpusCodec, error) {
	if parser == nil {
		parser = NewParser()
	}
	if len(messages) == 0 {
		return &CorpusCodec{
			templates: nil,
			entries:   map[string]codecEntry{},
		}, nil
	}

	_, templateSets, masks := parser.ParseWithTemplatesAndMasks(messages)
	matcher := NewMatcherFromTemplateSets(templateSets)
	templates := matcher.Templates()

	templateIDs := make(map[string]int, len(templates))
	for id, template := range templates {
		templateIDs[template] = id
	}
	ensureTemplateID := func(template string) int {
		if id, ok := templateIDs[template]; ok {
			return id
		}
		id := len(templates)
		templates = append(templates, template)
		templateIDs[template] = id
		return id
	}

	entries := make(map[string]codecEntry, len(messages))
	for i, msg := range messages {
		if len(masks[i]) != len(msg) {
			// Unclustered rows may have no mask. Keep them as literal templates so
			// every training-corpus message is encodable by exact lookup.
			id := ensureTemplateID(msg)
			prev, exists := entries[msg]
			if exists {
				if prev.templateID != id || len(prev.args) != 0 {
					return nil, fmt.Errorf("build codec from message %d: duplicate line has inconsistent literal encoding", i)
				}
				continue
			}
			entries[msg] = codecEntry{templateID: id, args: nil}
			continue
		}

		template, args, err := templateAndArgsFromMask(msg, masks[i])
		if err != nil {
			return nil, fmt.Errorf("build codec from message %d: %w", i, err)
		}

		id, ok := templateIDs[template]
		if !ok {
			// Safety fallback for edge cases where derived template string shape differs.
			id, ok = matcher.MatchID(msg)
			if !ok {
				id = ensureTemplateID(template)
			}
		}

		prev, exists := entries[msg]
		if exists {
			if prev.templateID != id || !equalStringSlices(prev.args, args) {
				return nil, fmt.Errorf("build codec from message %d: duplicate line has inconsistent encoding", i)
			}
			continue
		}

		entries[msg] = codecEntry{
			templateID: id,
			args:       append([]string(nil), args...),
		}
	}

	return &CorpusCodec{
		templates: templates,
		entries:   entries,
	}, nil
}

// Templates returns the codec template table.
func (c *CorpusCodec) Templates() []string {
	if c == nil || len(c.templates) == 0 {
		return nil
	}
	out := make([]string, len(c.templates))
	copy(out, c.templates)
	return out
}

// EncodeID resolves the template id for a message by exact message lookup.
// ok=false means the message was not in the training corpus.
func (c *CorpusCodec) EncodeID(message string) (templateID int, ok bool) {
	if c == nil {
		return -1, false
	}
	entry, ok := c.entries[message]
	if !ok {
		return -1, false
	}
	return entry.templateID, true
}

// Encode resolves a message to template id + args by exact message lookup.
// ok=false means the message was not in the training corpus.
func (c *CorpusCodec) Encode(message string) (templateID int, args []string, ok bool) {
	if c == nil {
		return -1, nil, false
	}
	entry, ok := c.entries[message]
	if !ok {
		return -1, nil, false
	}
	return entry.templateID, append([]string(nil), entry.args...), true
}

// Decode reconstructs a message from template id and args.
// ok=false means invalid template id or arg count mismatch.
func (c *CorpusCodec) Decode(templateID int, args []string) (message string, ok bool) {
	if c == nil || templateID < 0 || templateID >= len(c.templates) {
		return "", false
	}
	template := c.templates[templateID]
	parts := strings.Split(template, templatePlaceholder)
	if len(args) != len(parts)-1 {
		return "", false
	}

	var total int
	for _, part := range parts {
		total += len(part)
	}
	for _, arg := range args {
		total += len(arg)
	}

	var b strings.Builder
	b.Grow(total)
	for i, part := range parts {
		b.WriteString(part)
		if i < len(args) {
			b.WriteString(args[i])
		}
	}
	return b.String(), true
}

func templateAndArgsFromMask(message, mask string) (template string, args []string, err error) {
	if len(message) != len(mask) {
		return "", nil, fmt.Errorf("mask length %d does not match message length %d", len(mask), len(message))
	}

	out := make([]byte, 0, len(message))
	args = make([]string, 0, 4)
	for i := 0; i < len(message); {
		switch mask[i] {
		case '0':
			out = append(out, message[i])
			i++
		case '1':
			j := i + 1
			for j < len(message) && mask[j] == '1' {
				j++
			}
			out = append(out, templatePlaceholder...)
			args = append(args, message[i:j])
			i = j
		default:
			return "", nil, fmt.Errorf("invalid mask character %q at offset %d", mask[i], i)
		}
	}
	return string(out), args, nil
}

func equalStringSlices(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
