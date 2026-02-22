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
	groups := groupByAnchorTokens(tokenized, idep, p.threshold)

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
		richTokenizer := tokenizer.cloneWithSymbols(newSymbolSet(allPunctuationSymbols))
		richTokenized = tokenizeMessages(messages, richTokenizer)
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

func groupByAnchorTokens(
	tokenized [][]Token,
	idep *tokenRecord,
	threshold float64,
) []anchorGroup {
	if len(tokenized) == 0 {
		return nil
	}

	merged := make(map[string]*anchorGroup, len(tokenized))
	scratch := newAnchorScratch()
	for idx := range tokenized {
		anchors := anchorTokens(tokenized[idx], idep, threshold, scratch)
		key, ordered := canonicalAnchorSet(anchors)
		group, ok := merged[key]
		if !ok {
			group = &anchorGroup{
				key:     key,
				anchors: append([]Token(nil), ordered...),
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

func canonicalAnchorSet(anchors map[tokenKey]Token) (string, []Token) {
	if len(anchors) == 0 {
		return "", nil
	}
	keys := make([]tokenKey, 0, len(anchors))
	for k := range anchors {
		keys = append(keys, k)
	}
	sort.Sort(tokenKeyList(keys))

	ordered := make([]Token, len(keys))
	var b strings.Builder
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
