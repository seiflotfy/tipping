package tipping

import (
	"fmt"
	"math"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"sync"
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
	idep := newTokenRecord(messages, tokenizer, filter)
	groups := groupByAnchorTokens(messages, tokenizer, idep, p.threshold)

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

	var richTokenizer *Tokenizer
	if wantTemplates || wantMasks {
		richTokenizer = tokenizer.cloneWithSymbols(newSymbolSet(allPunctuationSymbols))
	}

	cid := 0
	for _, group := range groups {
		if len(group.anchors) == 0 {
			continue
		}

		clusterMsgs := make([]string, len(group.indices))
		for i, idx := range group.indices {
			clusterMsgs[i] = messages[idx]
		}

		if wantTemplates || wantMasks {
			shared := sharedSlices(clusterMsgs, richTokenizer, filter)
			if wantTemplates {
				templates = append(templates, templatesForCluster(clusterMsgs, richTokenizer, shared))
			}
			if wantMasks {
				clusterMasks := parameterMasks(clusterMsgs, richTokenizer, shared)
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
	messages []string,
	tokenizer *Tokenizer,
	idep *tokenRecord,
	threshold float64,
) []anchorGroup {
	if len(messages) == 0 {
		return nil
	}

	workers := runtime.GOMAXPROCS(0)
	if workers < 1 {
		workers = 1
	}
	if workers > len(messages) {
		workers = len(messages)
	}

	jobs := make(chan int)
	partials := make(chan map[string]*anchorGroup, workers)

	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			local := make(map[string]*anchorGroup)
			for idx := range jobs {
				tokens := tokenizer.Tokenize(messages[idx])
				anchors := anchorTokens(tokens, idep, threshold)
				key, ordered := canonicalAnchorSet(anchors)
				g, ok := local[key]
				if !ok {
					g = &anchorGroup{key: key, anchors: ordered}
					local[key] = g
				}
				g.indices = append(g.indices, idx)
			}
			partials <- local
		}()
	}

	go func() {
		for i := range messages {
			jobs <- i
		}
		close(jobs)
		wg.Wait()
		close(partials)
	}()

	merged := make(map[string]*anchorGroup)
	for partial := range partials {
		for key, group := range partial {
			existing, ok := merged[key]
			if !ok {
				merged[key] = &anchorGroup{
					key:     key,
					anchors: append([]Token(nil), group.anchors...),
					indices: append([]int(nil), group.indices...),
				}
				continue
			}
			existing.indices = append(existing.indices, group.indices...)
		}
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

func canonicalAnchorSet(anchors map[string]Token) (string, []Token) {
	if len(anchors) == 0 {
		return "", nil
	}
	ids := make([]string, 0, len(anchors))
	for id := range anchors {
		ids = append(ids, id)
	}
	sort.Strings(ids)

	ordered := make([]Token, len(ids))
	for i, id := range ids {
		ordered[i] = anchors[id]
	}
	return strings.Join(ids, "\x00"), ordered
}

func validateThreshold(value float64) error {
	if math.IsNaN(value) || math.IsInf(value, 0) || value < 0 || value > 1 {
		return fmt.Errorf("threshold must be in [0,1], got %v", value)
	}
	return nil
}
