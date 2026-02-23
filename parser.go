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
const defaultPatternSample = 1.0
const DefaultSymbols = "()[]{}=,*"

var allPunctuationSymbolSet = newSymbolSet(allPunctuationSymbols)
var defaultSymbolSet = newSymbolSet(DefaultSymbols)

// Parser is a Go implementation of Token Interdependency Parsing.
type Parser struct {
	threshold        float64
	patternSample    float64
	specialWhites    []*regexp.Regexp
	specialBlacks    []*regexp.Regexp
	symbols          map[rune]struct{}
	filterAlphabetic bool
	filterNumeric    bool
	filterImpure     bool
}

// ParseBuffers holds reusable memory for repeated parse calls.
// It is not safe for concurrent use.
type ParseBuffers struct {
	Clusters  []int
	Templates [][]string
	Masks     []string

	tokenized        [][]Token
	richTokenized    [][]Token
	clusterTokens    [][]Token
	tokenScratch     tokenizationScratch
	richTokenScratch tokenizationScratch
}

func NewParseBuffers() *ParseBuffers {
	return &ParseBuffers{}
}

// NewParser builds a parser with production-safe defaults.
func NewParser() *Parser {
	return &Parser{
		threshold:        defaultThreshold,
		patternSample:    defaultPatternSample,
		specialWhites:    nil,
		specialBlacks:    nil,
		symbols:          cloneSymbolSet(defaultSymbolSet),
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

// SetPatternSample validates and sets the fraction of messages used for pattern creation in (0,1].
func (p *Parser) SetPatternSample(value float64) error {
	if err := validatePatternSample(value); err != nil {
		return err
	}
	p.patternSample = value
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

// WithSymbols sets split symbols used in primary tokenization.
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
	clusters, _, _ := p.parse(messages, false, false, nil)
	return clusters
}

// ParseWithTemplates returns cluster IDs and per-cluster templates.
func (p *Parser) ParseWithTemplates(messages []string) ([]int, [][]string) {
	clusters, templates, _ := p.parse(messages, true, false, nil)
	return clusters, templates
}

// ParseWithMasks returns cluster IDs and per-message parameter masks.
func (p *Parser) ParseWithMasks(messages []string) ([]int, []string) {
	clusters, _, masks := p.parse(messages, false, true, nil)
	return clusters, masks
}

// ParseWithTemplatesAndMasks returns cluster IDs, templates, and masks.
func (p *Parser) ParseWithTemplatesAndMasks(messages []string) ([]int, [][]string, []string) {
	return p.parse(messages, true, true, nil)
}

func (p *Parser) ParseInto(messages []string, buffers *ParseBuffers) []int {
	clusters, _, _ := p.parse(messages, false, false, buffers)
	return clusters
}

func (p *Parser) ParseWithTemplatesInto(messages []string, buffers *ParseBuffers) ([]int, [][]string) {
	clusters, templates, _ := p.parse(messages, true, false, buffers)
	return clusters, templates
}

func (p *Parser) ParseWithMasksInto(messages []string, buffers *ParseBuffers) ([]int, []string) {
	clusters, _, masks := p.parse(messages, false, true, buffers)
	return clusters, masks
}

func (p *Parser) ParseWithTemplatesAndMasksInto(messages []string, buffers *ParseBuffers) ([]int, [][]string, []string) {
	return p.parse(messages, true, true, buffers)
}

func (p *Parser) parse(messages []string, wantTemplates, wantMasks bool, buffers *ParseBuffers) ([]int, [][]string, []string) {
	if len(messages) == 0 {
		if buffers != nil {
			buffers.Clusters = buffers.Clusters[:0]
			buffers.Templates = buffers.Templates[:0]
			buffers.Masks = buffers.Masks[:0]
			if wantMasks {
				return buffers.Clusters, buffers.Templates, buffers.Masks
			}
			if wantTemplates {
				return buffers.Clusters, buffers.Templates, nil
			}
			return buffers.Clusters, nil, nil
		}
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
	var tokenized [][]Token
	if buffers != nil {
		tokenized = tokenizeMessagesInto(messages, tokenizer, buffers.tokenized, &buffers.tokenScratch)
		buffers.tokenized = tokenized
	} else {
		tokenized = tokenizeMessages(messages, tokenizer)
	}
	idep := newTokenRecord(tokenized, filter)
	groups := groupByAnchorTokens(tokenized, idep, p.threshold, wantTemplates || wantMasks)

	var clusters []int
	if buffers != nil {
		clusters = ensureIntSliceWithValue(buffers.Clusters, len(messages), -1)
		buffers.Clusters = clusters
	} else {
		clusters = make([]int, len(messages))
		for i := range clusters {
			clusters[i] = -1
		}
	}

	var templates [][]string
	if wantTemplates {
		if buffers != nil {
			templates = buffers.Templates[:0]
		} else {
			templates = make([][]string, 0, len(groups))
		}
	}
	var masks []string
	if wantMasks {
		if buffers != nil {
			masks = ensureStringSlice(buffers.Masks, len(messages))
			clear(masks)
			buffers.Masks = masks
		} else {
			masks = make([]string, len(messages))
		}
	}

	var richTokenized [][]Token
	if wantTemplates || wantMasks {
		canReuse := symbolSetSubset(p.symbols, allPunctuationSymbolSet)
		if canReuse && symbolSetEqual(p.symbols, allPunctuationSymbolSet) {
			richTokenized = tokenized
		} else if canReuse {
			if buffers != nil {
				richTokenized = retokenizeMessagesWithSymbolsInto(tokenized, allPunctuationSymbolSet, buffers.richTokenized)
				buffers.richTokenized = richTokenized
			} else {
				richTokenized = retokenizeMessagesWithSymbols(tokenized, allPunctuationSymbolSet)
			}
		} else {
			richTokenizer := tokenizer.cloneWithSymbols(allPunctuationSymbolSet)
			if buffers != nil {
				richTokenized = tokenizeMessagesInto(messages, richTokenizer, buffers.richTokenized, &buffers.richTokenScratch)
				buffers.richTokenized = richTokenized
			} else {
				richTokenized = tokenizeMessages(messages, richTokenizer)
			}
		}
	}

	cid := 0
	for _, group := range groups {
		if len(group.anchors) == 0 {
			continue
		}

		if wantTemplates || wantMasks {
			var clusterTokens [][]Token
			if buffers != nil {
				if cap(buffers.clusterTokens) < len(group.indices) {
					buffers.clusterTokens = make([][]Token, len(group.indices))
				}
				clusterTokens = buffers.clusterTokens[:len(group.indices)]
				buffers.clusterTokens = clusterTokens
			} else {
				clusterTokens = make([][]Token, len(group.indices))
			}
			for i, idx := range group.indices {
				clusterTokens[i] = richTokenized[idx]
			}
			patternTokens := sampleTokenRows(clusterTokens, p.patternSample)
			shared := sharedSlices(patternTokens, filter)
			if wantTemplates {
				templates = append(templates, templatesForCluster(patternTokens, shared))
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

	if buffers != nil {
		buffers.Templates = templates
		buffers.Masks = masks
	}
	return clusters, templates, masks
}

func sampleTokenRows(tokenized [][]Token, sample float64) [][]Token {
	n := len(tokenized)
	if n <= 1 || sample >= 1 {
		return tokenized
	}

	sampleN := int(math.Ceil(float64(n) * sample))
	if sampleN < 1 {
		sampleN = 1
	}
	if sampleN >= n {
		return tokenized
	}
	if sampleN == 1 {
		return tokenized[:1]
	}

	out := make([][]Token, sampleN)
	step := float64(n-1) / float64(sampleN-1)
	prev := -1
	for i := 0; i < sampleN; i++ {
		idx := int(math.Round(float64(i) * step))
		if idx <= prev {
			idx = prev + 1
		}
		if idx >= n {
			idx = n - 1
		}
		out[i] = tokenized[idx]
		prev = idx
	}
	return out
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
	return tokenizeMessagesInto(messages, tokenizer, nil, nil)
}

func tokenizeMessagesInto(
	messages []string,
	tokenizer *Tokenizer,
	dst [][]Token,
	scratch *tokenizationScratch,
) [][]Token {
	tokenized := dst
	if cap(tokenized) < len(messages) {
		tokenized = make([][]Token, len(messages))
	} else {
		tokenized = tokenized[:len(messages)]
	}
	for i := range messages {
		tokenized[i] = tokenizer.TokenizeInto(messages[i], tokenized[i], scratch)
	}
	return tokenized
}

func retokenizeMessagesWithSymbols(tokenized [][]Token, symbols map[rune]struct{}) [][]Token {
	return retokenizeMessagesWithSymbolsInto(tokenized, symbols, nil)
}

func retokenizeMessagesWithSymbolsInto(
	tokenized [][]Token,
	symbols map[rune]struct{},
	dst [][]Token,
) [][]Token {
	out := dst
	if cap(out) < len(tokenized) {
		out = make([][]Token, len(tokenized))
	} else {
		out = out[:len(tokenized)]
	}
	for i := range tokenized {
		out[i] = retokenizeTokensWithSymbolsInto(tokenized[i], symbols, out[i])
	}
	return out
}

func retokenizeTokensWithSymbols(tokens []Token, symbols map[rune]struct{}) []Token {
	return retokenizeTokensWithSymbolsInto(tokens, symbols, nil)
}

func retokenizeTokensWithSymbolsInto(tokens []Token, symbols map[rune]struct{}, dst []Token) []Token {
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

	out := dst[:0]
	needed := len(tokens)
	for _, tok := range tokens[start:] {
		if tok.Kind != TokenImpure {
			continue
		}
		splitCount := splitTokenCount(tok.Slice, symbols)
		if splitCount > 1 {
			needed += splitCount - 1
		}
	}
	if cap(out) < needed {
		out = make([]Token, 0, needed)
	}
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

func ensureIntSliceWithValue(buf []int, n int, value int) []int {
	if cap(buf) < n {
		buf = make([]int, n)
	} else {
		buf = buf[:n]
	}
	for i := range buf {
		buf[i] = value
	}
	return buf
}

func ensureStringSlice(buf []string, n int) []string {
	if cap(buf) < n {
		return make([]string, n)
	}
	return buf[:n]
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

func validatePatternSample(value float64) error {
	if math.IsNaN(value) || math.IsInf(value, 0) || value <= 0 || value > 1 {
		return fmt.Errorf("pattern sample must be in (0,1], got %v", value)
	}
	return nil
}
