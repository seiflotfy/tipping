package tipping

import (
	"sort"
	"strings"
)

const templatePlaceholder = "<*>"

// Matcher performs runtime string-to-template matching.
type Matcher struct {
	templates []string

	byID        []compiledTemplate
	rankByID    []int
	exact       map[string]int
	prefixIndex map[string][]int
	suffixIndex map[string][]int
	middleIndex map[string][]int
	middleKeys  []string
	maxPrefix   int
	maxSuffix   int
}

type compiledTemplate struct {
	id               int
	template         string
	parts            []string
	placeholderCount int
	literalBytes     int
	firstLiteral     string
	lastLiteral      string
}

// NewMatcher builds a matcher from a flat template list.
func NewMatcher(templates []string) *Matcher {
	entries := make([]compiledTemplate, 0, len(templates))
	kept := make([]string, 0, len(templates))
	seen := make(map[string]int, len(templates))
	for _, raw := range templates {
		tmpl := normalizeTemplate(raw)
		if _, ok := seen[tmpl]; ok {
			continue
		}
		id := len(kept)
		kept = append(kept, tmpl)
		seen[tmpl] = id
		entries = append(entries, compileTemplate(id, tmpl))
	}

	order := make([]int, len(entries))
	for i := range entries {
		order[i] = i
	}
	sort.SliceStable(order, func(i, j int) bool {
		ei := entries[order[i]]
		ej := entries[order[j]]
		if ei.literalBytes != ej.literalBytes {
			return ei.literalBytes > ej.literalBytes
		}
		if ei.placeholderCount != ej.placeholderCount {
			return ei.placeholderCount < ej.placeholderCount
		}
		return ei.id < ej.id
	})
	rankByID := make([]int, len(entries))
	for rank, id := range order {
		rankByID[id] = rank
	}

	exact := make(map[string]int)
	prefixIndex := make(map[string][]int)
	suffixIndex := make(map[string][]int)
	middleIndex := make(map[string][]int)
	middleKeys := make([]string, 0)
	maxPrefix := 0
	maxSuffix := 0
	for _, entry := range entries {
		if entry.placeholderCount == 0 {
			exact[entry.template] = entry.id
			continue
		}
		if entry.firstLiteral != "" {
			prefixIndex[entry.firstLiteral] = append(prefixIndex[entry.firstLiteral], entry.id)
			if len(entry.firstLiteral) > maxPrefix {
				maxPrefix = len(entry.firstLiteral)
			}
			continue
		}
		if entry.lastLiteral != "" {
			suffixIndex[entry.lastLiteral] = append(suffixIndex[entry.lastLiteral], entry.id)
			if len(entry.lastLiteral) > maxSuffix {
				maxSuffix = len(entry.lastLiteral)
			}
			continue
		}
		anchor := strongestMiddleLiteral(entry.parts)
		if anchor == "" {
			continue
		}
		if _, ok := middleIndex[anchor]; !ok {
			middleKeys = append(middleKeys, anchor)
		}
		middleIndex[anchor] = append(middleIndex[anchor], entry.id)
	}
	sortIDsByRank(prefixIndex, rankByID)
	sortIDsByRank(suffixIndex, rankByID)
	sortIDsByRank(middleIndex, rankByID)
	sort.Slice(middleKeys, func(i, j int) bool {
		if len(middleKeys[i]) != len(middleKeys[j]) {
			return len(middleKeys[i]) > len(middleKeys[j])
		}
		return middleKeys[i] < middleKeys[j]
	})

	return &Matcher{
		templates:   kept,
		byID:        entries,
		rankByID:    rankByID,
		exact:       exact,
		prefixIndex: prefixIndex,
		suffixIndex: suffixIndex,
		middleIndex: middleIndex,
		middleKeys:  middleKeys,
		maxPrefix:   maxPrefix,
		maxSuffix:   maxSuffix,
	}
}

// NewMatcherFromTemplateSets flattens parser template sets and builds a matcher.
func NewMatcherFromTemplateSets(templateSets [][]string) *Matcher {
	flat := make([]string, 0)
	for _, set := range templateSets {
		flat = append(flat, set...)
	}
	return NewMatcher(flat)
}

// Templates returns the matcher template table.
func (m *Matcher) Templates() []string {
	if m == nil || len(m.templates) == 0 {
		return nil
	}
	out := make([]string, len(m.templates))
	copy(out, m.templates)
	return out
}

// Match returns the best-matching template id and extracted args.
// ok=false means no template matched.
func (m *Matcher) Match(msg string) (templateID int, args []string, ok bool) {
	id, ok := m.MatchID(msg)
	if !ok {
		return -1, nil, false
	}
	args, ok = m.byID[id].match(msg)
	if !ok {
		return -1, nil, false
	}
	return id, args, true
}

// MatchID returns only the best-matching template id.
// ok=false means no template matched.
func (m *Matcher) MatchID(msg string) (templateID int, ok bool) {
	id, ok := m.bestMatchID(msg)
	if !ok {
		return -1, false
	}
	return id, true
}

// MatchAllIDs matches a batch of messages and returns template ids per input index.
// Duplicate messages are resolved once and reused.
func (m *Matcher) MatchAllIDs(messages []string) []int {
	ids := make([]int, len(messages))
	for i := range ids {
		ids[i] = -1
	}
	if m == nil || len(messages) == 0 {
		return ids
	}

	cacheCap := len(messages)
	if cacheCap > 1<<16 {
		cacheCap = 1 << 16
	}
	cache := make(map[string]int, cacheCap)
	for i, msg := range messages {
		if id, ok := cache[msg]; ok {
			ids[i] = id
			continue
		}
		id, ok := m.MatchID(msg)
		if !ok {
			cache[msg] = -1
			continue
		}
		ids[i] = id
		cache[msg] = id
	}
	return ids
}

func (m *Matcher) bestMatchID(msg string) (int, bool) {
	if m == nil {
		return -1, false
	}

	if id, ok := m.exact[msg]; ok {
		return id, true
	}

	bestID := -1
	bestRank := len(m.byID) + 1
	evaluate := func(id int) {
		rank := m.rankByID[id]
		if rank >= bestRank {
			return
		}
		if !m.byID[id].matches(msg) {
			return
		}
		bestID = id
		bestRank = rank
	}

	if m.maxPrefix > 0 && len(msg) > 0 {
		limit := m.maxPrefix
		if len(msg) < limit {
			limit = len(msg)
		}
		for n := 1; n <= limit; n++ {
			candidates, ok := m.prefixIndex[msg[:n]]
			if !ok {
				continue
			}
			for _, id := range candidates {
				evaluate(id)
			}
		}
	}

	if m.maxSuffix > 0 && len(msg) > 0 {
		limit := m.maxSuffix
		if len(msg) < limit {
			limit = len(msg)
		}
		for n := 1; n <= limit; n++ {
			start := len(msg) - n
			candidates, ok := m.suffixIndex[msg[start:]]
			if !ok {
				continue
			}
			for _, id := range candidates {
				evaluate(id)
			}
		}
	}

	for _, anchor := range m.middleKeys {
		if !strings.Contains(msg, anchor) {
			continue
		}
		candidates := m.middleIndex[anchor]
		for _, id := range candidates {
			evaluate(id)
		}
	}

	if bestID >= 0 {
		return bestID, true
	}
	return -1, false
}

func compileTemplate(id int, tmpl string) compiledTemplate {
	parts := strings.Split(tmpl, templatePlaceholder)
	literalBytes := 0
	for _, p := range parts {
		literalBytes += len(p)
	}
	return compiledTemplate{
		id:               id,
		template:         tmpl,
		parts:            parts,
		placeholderCount: len(parts) - 1,
		literalBytes:     literalBytes,
		firstLiteral:     parts[0],
		lastLiteral:      parts[len(parts)-1],
	}
}

func (t compiledTemplate) match(msg string) ([]string, bool) { return t.matchInternal(msg, true) }

func (t compiledTemplate) matches(msg string) bool {
	_, ok := t.matchInternal(msg, false)
	return ok
}

func (t compiledTemplate) matchInternal(msg string, capture bool) ([]string, bool) {
	if t.placeholderCount == 0 {
		return nil, msg == t.template
	}

	cursor := 0
	first := t.parts[0]
	if first != "" {
		if !strings.HasPrefix(msg, first) {
			return nil, false
		}
		cursor = len(first)
	}

	var args []string
	if capture {
		args = make([]string, 0, t.placeholderCount)
	}
	for i := 0; i < t.placeholderCount; i++ {
		next := t.parts[i+1]
		last := i == t.placeholderCount-1

		if last {
			if next == "" {
				if cursor >= len(msg) {
					return nil, false
				}
				if capture {
					args = append(args, msg[cursor:])
				}
				cursor = len(msg)
				break
			}
			if !strings.HasSuffix(msg, next) {
				return nil, false
			}
			end := len(msg) - len(next)
			if end <= cursor {
				return nil, false
			}
			if capture {
				args = append(args, msg[cursor:end])
			}
			cursor = end + len(next)
			break
		}

		if next == "" {
			return nil, false
		}
		idx := strings.Index(msg[cursor:], next)
		if idx < 0 {
			return nil, false
		}
		if idx == 0 {
			return nil, false
		}
		if capture {
			args = append(args, msg[cursor:cursor+idx])
		}
		cursor += idx + len(next)
	}

	return args, cursor == len(msg)
}

func normalizeTemplate(tmpl string) string {
	if tmpl == "" {
		return ""
	}
	repeat := templatePlaceholder + templatePlaceholder
	for strings.Contains(tmpl, repeat) {
		tmpl = strings.ReplaceAll(tmpl, repeat, templatePlaceholder)
	}
	return tmpl
}

func sortIDsByRank(index map[string][]int, rankByID []int) {
	for key, ids := range index {
		sort.Slice(ids, func(i, j int) bool {
			return rankByID[ids[i]] < rankByID[ids[j]]
		})
		index[key] = ids
	}
}

func strongestMiddleLiteral(parts []string) string {
	if len(parts) <= 2 {
		return ""
	}
	best := ""
	for i := 1; i < len(parts)-1; i++ {
		part := parts[i]
		if len(part) > len(best) {
			best = part
		}
	}
	return best
}
