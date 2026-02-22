package tipping

import (
	"fmt"
	"math"
	"regexp"
	"sort"
	"strings"
)

const allPunctuationSymbols = "!\"#$%&'()*+,-./:;<=>?@[\\]^_`{|}~"
const defaultThreshold = 0.5

var allPunctuationSymbolSet = newSymbolSet(allPunctuationSymbols)

// Parser is a Go implementation of Token Interdependency Parsing.
type Parser struct {
	threshold        float64
	specialWhites    []*regexp.Regexp
	specialBlacks    []*regexp.Regexp
	symbols          map[rune]struct{}
	filterAlphabetic bool
	filterNumeric    bool
	filterImpure     bool
}

// NewParser builds a parser with production-safe defaults.
func NewParser() *Parser {
	return &Parser{
		threshold:        defaultThreshold,
		specialWhites:    nil,
		specialBlacks:    nil,
		symbols:          map[rune]struct{}{},
		filterAlphabetic: true,
		filterNumeric:    false,
		filterImpure:     false,
	}
}

// SetThreshold validates and sets the interdependency threshold in [0,1].
func (p *Parser) SetThreshold(value float64) error {
	if err := validateThreshold(value); err != nil {
		return err
	}
	p.threshold = value
	return nil
}

// WithSpecialWhites sets regexes that are never parameterized.
func (p *Parser) WithSpecialWhites(value []*regexp.Regexp) *Parser {
	p.specialWhites = append([]*regexp.Regexp(nil), value...)
	return p
}

// WithSpecialBlacks sets regexes that are always parameterized.
func (p *Parser) WithSpecialBlacks(value []*regexp.Regexp) *Parser {
	p.specialBlacks = append([]*regexp.Regexp(nil), value...)
	return p
}

// WithSymbols sets extra split symbols used in primary tokenization.
func (p *Parser) WithSymbols(symbols string) *Parser {
	p.symbols = newSymbolSet(symbols)
	return p
}

// WithFilterAlphabetic controls alphabetic token participation in dependency scoring.
func (p *Parser) WithFilterAlphabetic(value bool) *Parser {
	p.filterAlphabetic = value
	return p
}

// WithFilterNumeric controls numeric token participation in dependency scoring.
func (p *Parser) WithFilterNumeric(value bool) *Parser {
	p.filterNumeric = value
	return p
}

// WithFilterImpure controls impure token participation in dependency scoring.
func (p *Parser) WithFilterImpure(value bool) *Parser {
	p.filterImpure = value
	return p
}

// Parse returns cluster IDs (-1 means unclustered).
func (p *Parser) Parse(messages []string) []int {
	clusters, _, _ := p.parse(messages, false, false)
	return clusters
}

// ParseWithTemplates returns cluster IDs and per-cluster templates.
func (p *Parser) ParseWithTemplates(messages []string) ([]int, [][]string) {
	clusters, templates, _ := p.parse(messages, true, false)
	return clusters, templates
}

// ParseWithMasks returns cluster IDs and per-message parameter masks.
func (p *Parser) ParseWithMasks(messages []string) ([]int, []string) {
	clusters, _, masks := p.parse(messages, false, true)
	return clusters, masks
}

// ParseWithTemplatesAndMasks returns cluster IDs, templates, and masks.
func (p *Parser) ParseWithTemplatesAndMasks(messages []string) ([]int, [][]string, []string) {
	return p.parse(messages, true, true)
}

func (p *Parser) parse(messages []string, wantTemplates, wantMasks bool) ([]int, [][]string, []string) {
	if len(messages) == 0 {
		if wantMasks {
			return []int{}, [][]string{}, []string{}
		}
		if wantTemplates {
			return []int{}, [][]string{}, nil
		}
		return []int{}, nil, nil
	}

	tokenizer := NewTokenizer(p.specialWhites, p.specialBlacks, p.symbols)
	filter := newStaticFilter(p.filterAlphabetic, p.filterNumeric, p.filterImpure)
	tokenized := tokenizeMessages(messages, tokenizer)
	idep := newTokenRecord(tokenized, filter)
	groups := groupByAnchorTokens(tokenized, idep, p.threshold, wantTemplates || wantMasks)

	clusters := make([]int, len(messages))
	for i := range clusters {
		clusters[i] = -1
	}

	var templates [][]string
	if wantTemplates {
		templates = make([][]string, 0, len(groups))
	}
	var masks []string
	if wantMasks {
		masks = make([]string, len(messages))
	}

	var richTokenized [][]Token
	if wantTemplates || wantMasks {
		canReuse := symbolSetSubset(p.symbols, allPunctuationSymbolSet)
		if canReuse && symbolSetEqual(p.symbols, allPunctuationSymbolSet) {
			richTokenized = tokenized
		} else if canReuse {
			richTokenized = retokenizeMessagesWithSymbols(tokenized, allPunctuationSymbolSet)
		} else {
			richTokenizer := tokenizer.cloneWithSymbols(allPunctuationSymbolSet)
			richTokenized = tokenizeMessages(messages, richTokenizer)
		}
	}

	cid := 0
	for _, group := range groups {
		if len(group.anchors) == 0 {
			continue
		}

		if wantTemplates || wantMasks {
			clusterTokens := make([][]Token, len(group.indices))
			for i, idx := range group.indices {
				clusterTokens[i] = richTokenized[idx]
			}
			shared := sharedSlices(clusterTokens, filter)
			if wantTemplates {
				templates = append(templates, templatesForCluster(clusterTokens, shared))
			}
			if wantMasks {
				clusterMasks := parameterMasks(clusterTokens, shared)
				for i, idx := range group.indices {
					masks[idx] = clusterMasks[i]
				}
			}
		}

		for _, idx := range group.indices {
			clusters[idx] = cid
		}
		cid++
	}

	return clusters, templates, masks
}

type anchorGroup struct {
	key     string
	anchors []Token
	indices []int
}

type canonicalScratch struct {
	keys    []tokenKey
	ordered []Token
}

func groupByAnchorTokens(
	tokenized [][]Token,
	idep *tokenRecord,
	threshold float64,
	lowAlloc bool,
) []anchorGroup {
	if len(tokenized) == 0 {
		return nil
	}

	merged := make(map[string]*anchorGroup, len(tokenized))
	scratch := newAnchorScratch()
	canonical := &canonicalScratch{}
	for idx := range tokenized {
		anchors := anchorTokens(tokenized[idx], idep, threshold, scratch)
		var key string
		var ordered []Token
		if lowAlloc {
			key, ordered = canonicalAnchorSet(anchors, canonical)
		} else {
			key, ordered = canonicalAnchorSetFast(anchors)
		}
		group, ok := merged[key]
		if !ok {
			groupAnchors := make([]Token, len(ordered))
			copy(groupAnchors, ordered)
			group = &anchorGroup{
				key:     key,
				anchors: groupAnchors,
			}
			merged[key] = group
		}
		group.indices = append(group.indices, idx)
	}

	groups := make([]anchorGroup, 0, len(merged))
	for _, group := range merged {
		sort.Ints(group.indices)
		groups = append(groups, *group)
	}
	sort.Slice(groups, func(i, j int) bool {
		return groups[i].key < groups[j].key
	})

	return groups
}

func tokenizeMessages(messages []string, tokenizer *Tokenizer) [][]Token {
	tokenized := make([][]Token, len(messages))
	for i := range messages {
		tokenized[i] = tokenizer.Tokenize(messages[i])
	}
	return tokenized
}

func retokenizeMessagesWithSymbols(tokenized [][]Token, symbols map[rune]struct{}) [][]Token {
	out := make([][]Token, len(tokenized))
	for i := range tokenized {
		out[i] = retokenizeTokensWithSymbols(tokenized[i], symbols)
	}
	return out
}

func retokenizeTokensWithSymbols(tokens []Token, symbols map[rune]struct{}) []Token {
	start := -1
	for i, tok := range tokens {
		if tok.Kind == TokenImpure && tokenNeedsSplit(tok.Slice, symbols) {
			start = i
			break
		}
	}
	if start < 0 {
		return tokens
	}

	out := make([]Token, 0, len(tokens)*2)
	out = append(out, tokens[:start]...)
	for _, tok := range tokens[start:] {
		switch tok.Kind {
		case TokenSpecialWhite, TokenSpecialBlack, TokenWhitespace, TokenSymbolic, TokenAlphabetic, TokenNumeric:
			out = append(out, tok)
		default:
			out = appendSplitToken(out, tok.Slice, symbols)
		}
	}
	return out
}

func tokenNeedsSplit(slice string, symbols map[rune]struct{}) bool {
	for _, r := range slice {
		if _, ok := symbols[r]; ok {
			return true
		}
	}
	return false
}

func symbolSetSubset(sub, sup map[rune]struct{}) bool {
	for r := range sub {
		if _, ok := sup[r]; !ok {
			return false
		}
	}
	return true
}

func symbolSetEqual(a, b map[rune]struct{}) bool {
	if len(a) != len(b) {
		return false
	}
	return symbolSetSubset(a, b)
}

func canonicalAnchorSet(anchors map[tokenKey]Token, scratch *canonicalScratch) (string, []Token) {
	if len(anchors) == 0 {
		scratch.keys = scratch.keys[:0]
		scratch.ordered = scratch.ordered[:0]
		return "", scratch.ordered
	}
	keys := scratch.keys[:0]
	totalLen := 0
	for k := range anchors {
		keys = append(keys, k)
		totalLen += len(k.slice) + 3
	}
	sort.Sort(tokenKeyList(keys))
	scratch.keys = keys

	ordered := scratch.ordered
	if cap(ordered) < len(keys) {
		ordered = make([]Token, len(keys))
	} else {
		ordered = ordered[:len(keys)]
	}

	var b strings.Builder
	b.Grow(totalLen)
	for i, k := range keys {
		ordered[i] = anchors[k]
		b.WriteByte(byte(k.kind))
		b.WriteByte('\x1f')
		b.WriteString(k.slice)
		b.WriteByte('\x00')
	}
	scratch.ordered = ordered
	return b.String(), ordered
}

func canonicalAnchorSetFast(anchors map[tokenKey]Token) (string, []Token) {
	if len(anchors) == 0 {
		return "", nil
	}
	keys := make([]tokenKey, 0, len(anchors))
	totalLen := 0
	for k := range anchors {
		keys = append(keys, k)
		totalLen += len(k.slice) + 3
	}
	sort.Sort(tokenKeyList(keys))

	ordered := make([]Token, len(keys))
	var b strings.Builder
	b.Grow(totalLen)
	for i, k := range keys {
		ordered[i] = anchors[k]
		b.WriteByte(byte(k.kind))
		b.WriteByte('\x1f')
		b.WriteString(k.slice)
		b.WriteByte('\x00')
	}
	return b.String(), ordered
}

type tokenKeyList []tokenKey

func (l tokenKeyList) Len() int      { return len(l) }
func (l tokenKeyList) Swap(i, j int) { l[i], l[j] = l[j], l[i] }
func (l tokenKeyList) Less(i, j int) bool {
	if l[i].kind != l[j].kind {
		return l[i].kind < l[j].kind
	}
	return l[i].slice < l[j].slice
}

func validateThreshold(value float64) error {
	if math.IsNaN(value) || math.IsInf(value, 0) || value < 0 || value > 1 {
		return fmt.Errorf("threshold must be in [0,1], got %v", value)
	}
	return nil
}
