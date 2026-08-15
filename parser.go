package tipping

import (
	"cmp"
	"fmt"
	"math"
	"regexp"
	"runtime"
	"slices"
	"sort"
	"sync"
	"unicode/utf8"
)

const allPunctuationSymbols = "!\"#$%&'()*+,-./:;<=>?@[\\]^_`{|}~"
const defaultThreshold = 0.5
const defaultPatternSample = 1.0
const DefaultSymbols = "()[]{}=,*"

var allPunctuationSymbolSet = newSymbolSet(allPunctuationSymbols)
var allPunctuationSymbolTable = newSymbolTable(allPunctuationSymbolSet)
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
	parallelism      int
}

// Parallel stages engage only when there is enough independent work per
// goroutine to amortize spawn and merge cost.
const parallelRowsPerWorker = 512

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

	lineIndex  map[string]int32
	rowOf      []int32
	rowPos     []int32
	uniqueMsgs []string
	counts     []uint32

	interner    map[string]uint32
	idsFlat     []uint32
	rowIDs      [][]uint32
	richIDsFlat []uint32
	richRowIDs  [][]uint32
	clusterIDs  [][]uint32
	common      commonScratch
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

// WithParallelism bounds worker goroutines for the per-line parse stages.
// 0 (the default) uses GOMAXPROCS; 1 disables parallelism. Output is
// identical at any setting.
func (p *Parser) WithParallelism(n int) *Parser {
	p.parallelism = n
	return p
}

func (p *Parser) workersFor(rows int) int {
	limit := p.parallelism
	if limit <= 0 {
		limit = runtime.GOMAXPROCS(0)
	}
	byWork := rows / parallelRowsPerWorker
	if byWork < limit {
		limit = byWork
	}
	if limit < 2 {
		return 1
	}
	return limit
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
	if buffers == nil {
		buffers = NewParseBuffers()
	}
	// Unique line ids are int32; message count bounds them, so oversized
	// input is a defined, documented panic rather than silent truncation.
	if len(messages) > math.MaxInt32 {
		panic(fmt.Sprintf("tipping: parse: %d messages exceeds the supported maximum of %d", len(messages), math.MaxInt32))
	}

	tokenizer := NewTokenizer(p.specialWhites, p.specialBlacks, p.symbols)
	filter := newStaticFilter(p.filterAlphabetic, p.filterNumeric, p.filterImpure)

	// Deduplicate identical lines so every per-line stage below (tokenize,
	// count, anchor, key) runs once per unique line; occurrence counts are
	// weighted by duplicate count, which is arithmetically identical.
	if buffers.lineIndex == nil {
		buffers.lineIndex = make(map[string]int32, len(messages))
	} else {
		clear(buffers.lineIndex)
	}
	lineIndex := buffers.lineIndex
	rowOf := ensureInt32Slice(buffers.rowOf, len(messages))
	uniqueMsgs := buffers.uniqueMsgs[:0]
	counts := buffers.counts[:0]
	for i, msg := range messages {
		id, ok := lineIndex[msg]
		if !ok {
			id = int32(len(uniqueMsgs))
			lineIndex[msg] = id
			uniqueMsgs = append(uniqueMsgs, msg)
			counts = append(counts, 0)
		}
		counts[id]++
		rowOf[i] = id
	}
	buffers.rowOf = rowOf
	buffers.uniqueMsgs = uniqueMsgs
	buffers.counts = counts

	workers := p.workersFor(len(uniqueMsgs))
	var tokenized [][]Token
	if workers > 1 {
		tokenized = tokenizeMessagesParallelInto(uniqueMsgs, tokenizer, buffers.tokenized, workers)
	} else {
		tokenized = tokenizeMessagesInto(uniqueMsgs, tokenizer, buffers.tokenized, &buffers.tokenScratch)
	}
	buffers.tokenized = tokenized

	// Assign a dense id per distinct token slice once; every later stage
	// works on ids (array indexing) instead of re-hashing token strings.
	if buffers.interner == nil {
		buffers.interner = make(map[string]uint32)
	} else {
		clear(buffers.interner)
	}
	interner := buffers.interner
	var rowIDs [][]uint32
	buffers.idsFlat, rowIDs = internRows(tokenized, interner, buffers.idsFlat, buffers.rowIDs)
	buffers.rowIDs = rowIDs
	numPrimaryIDs := len(interner)

	idep := newTokenRecord(tokenized, rowIDs, counts, numPrimaryIDs, filter)
	groups := groupByAnchorTokens(tokenized, rowIDs, numPrimaryIDs, rowOf, idep, p.threshold, workers)

	clusters := ensureIntSliceWithValue(buffers.Clusters, len(messages), -1)
	buffers.Clusters = clusters

	var templates [][]string
	if wantTemplates {
		templates = buffers.Templates[:0]
	}
	var masks []string
	if wantMasks {
		masks = ensureStringSlice(buffers.Masks, len(messages))
		clear(masks)
		buffers.Masks = masks
	}

	var richTokenized [][]Token
	var richRowIDs [][]uint32
	if wantTemplates || wantMasks {
		canReuse := symbolSetSubset(p.symbols, allPunctuationSymbolSet)
		if canReuse && symbolSetEqual(p.symbols, allPunctuationSymbolSet) {
			richTokenized = tokenized
			richRowIDs = rowIDs
		} else {
			if canReuse {
				richTokenized = retokenizeMessagesWithSymbolsInto(tokenized, allPunctuationSymbolTable, buffers.richTokenized, workers)
			} else {
				richTokenizer := tokenizer.cloneWithSymbols(allPunctuationSymbolTable)
				if workers > 1 {
					richTokenized = tokenizeMessagesParallelInto(uniqueMsgs, richTokenizer, buffers.richTokenized, workers)
				} else {
					richTokenized = tokenizeMessagesInto(uniqueMsgs, richTokenizer, buffers.richTokenized, &buffers.richTokenScratch)
				}
			}
			buffers.richTokenized = richTokenized
			buffers.richIDsFlat, richRowIDs = internRows(richTokenized, interner, buffers.richIDsFlat, buffers.richRowIDs)
			buffers.richRowIDs = richRowIDs
		}
	}

	cid := 0
	var rowPos []int32
	if wantMasks {
		rowPos = ensureInt32Slice(buffers.rowPos, len(uniqueMsgs))
		buffers.rowPos = rowPos
	}
	var commonScr *commonScratch
	if wantTemplates || wantMasks {
		commonScr = &buffers.common
	}
	for _, group := range groups {
		if len(group.anchors) == 0 {
			continue
		}

		if wantTemplates || wantMasks {
			if cap(buffers.clusterTokens) < len(group.rows) {
				buffers.clusterTokens = make([][]Token, len(group.rows))
			}
			rowTokens := buffers.clusterTokens[:len(group.rows)]
			buffers.clusterTokens = rowTokens
			if cap(buffers.clusterIDs) < len(group.rows) {
				buffers.clusterIDs = make([][]uint32, len(group.rows))
			}
			rowIDsCluster := buffers.clusterIDs[:len(group.rows)]
			buffers.clusterIDs = rowIDsCluster
			for k, row := range group.rows {
				rowTokens[k] = richTokenized[row]
				rowIDsCluster[k] = richRowIDs[row]
			}

			patternTokens, patternIDs := rowTokens, rowIDsCluster
			if p.patternSample < 1 {
				srcTokens, srcIDs := rowTokens, rowIDsCluster
				if len(group.indices) > len(group.rows) {
					// Sub-sampling is defined over per-message rows; expand
					// duplicates so sampling picks the same rows as before dedup.
					srcTokens = make([][]Token, len(group.indices))
					srcIDs = make([][]uint32, len(group.indices))
					for i, idx := range group.indices {
						srcTokens[i] = richTokenized[rowOf[idx]]
						srcIDs[i] = richRowIDs[rowOf[idx]]
					}
				}
				patternTokens, patternIDs = srcTokens, srcIDs
				if pos := samplePositions(len(srcTokens), p.patternSample); pos != nil {
					patternTokens = make([][]Token, len(pos))
					patternIDs = make([][]uint32, len(pos))
					for k, src := range pos {
						patternTokens[k] = srcTokens[src]
						patternIDs[k] = srcIDs[src]
					}
				}
			}
			shared := sharedSlices(patternTokens, patternIDs, filter, commonScr, len(interner))
			if wantTemplates {
				templates = append(templates, templatesForCluster(patternTokens, patternIDs, shared))
			}
			if wantMasks {
				rowMasks := parameterMasks(rowTokens, rowIDsCluster, shared)
				for k, row := range group.rows {
					rowPos[row] = int32(k)
				}
				for _, idx := range group.indices {
					masks[idx] = rowMasks[rowPos[rowOf[idx]]]
				}
			}
		}

		for _, idx := range group.indices {
			clusters[idx] = cid
		}
		cid++
	}

	buffers.Templates = templates
	buffers.Masks = masks
	return clusters, templates, masks
}

// samplePositions returns evenly spaced row indices covering ceil(n*sample)
// rows, or nil when every row is taken.
func samplePositions(n int, sample float64) []int {
	if n <= 1 || sample >= 1 {
		return nil
	}

	sampleN := int(math.Ceil(float64(n) * sample))
	if sampleN < 1 {
		sampleN = 1
	}
	if sampleN >= n {
		return nil
	}
	if sampleN == 1 {
		return []int{0}
	}

	out := make([]int, sampleN)
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
		out[i] = idx
		prev = idx
	}
	return out
}

// internRows assigns dense ids to token slices, reusing flat as the backing
// arena and rows as the per-row index. interner persists across both primary
// and rich tokenization so ids are shared.
func internRows(tokenized [][]Token, interner map[string]uint32, flat []uint32, rows [][]uint32) ([]uint32, [][]uint32) {
	total := 0
	for _, toks := range tokenized {
		total += len(toks)
	}
	if cap(flat) < total {
		flat = make([]uint32, 0, total)
	} else {
		flat = flat[:0]
	}
	if cap(rows) < len(tokenized) {
		rows = make([][]uint32, len(tokenized))
	} else {
		rows = rows[:len(tokenized)]
	}
	for i, toks := range tokenized {
		off := len(flat)
		for _, tok := range toks {
			id, ok := interner[tok.Slice]
			if !ok {
				id = uint32(len(interner))
				interner[tok.Slice] = id
			}
			flat = append(flat, id)
		}
		rows[i] = flat[off:len(flat):len(flat)]
	}
	return flat, rows
}

type anchorGroup struct {
	key     string
	anchors []Token
	rows    []int32 // unique row ids, in first-appearance order
	indices []int   // original message indices, ascending
}

type canonicalScratch struct {
	keys    []tokenKey
	ordered []Token
	keyBuf  []byte
}

// anchorShard holds one worker's per-row canonical keys and ordered anchors,
// concatenated in arenas with offset tables.
type anchorShard struct {
	lo, hi int
	keys   []byte
	keyOff []int
	ancs   []Token
	ancOff []int
}

func groupByAnchorTokens(
	tokenized [][]Token,
	rowIDs [][]uint32,
	numIDs int,
	rowOf []int32,
	idep *tokenRecord,
	threshold float64,
	workers int,
) []anchorGroup {
	if len(tokenized) == 0 {
		return nil
	}

	merged := make(map[string]*anchorGroup, len(tokenized))
	rowGroup := make([]*anchorGroup, len(tokenized))

	addRow := func(row int, keyBytes []byte, ordered []Token) {
		group, ok := merged[string(keyBytes)]
		if !ok {
			groupAnchors := make([]Token, len(ordered))
			copy(groupAnchors, ordered)
			key := string(keyBytes)
			group = &anchorGroup{
				key:     key,
				anchors: groupAnchors,
			}
			merged[key] = group
		}
		group.rows = append(group.rows, int32(row))
		rowGroup[row] = group
	}

	if workers > 1 {
		// Anchor and key computation per row is independent; compute in
		// parallel shards, then merge serially in row order so grouping is
		// identical to the serial path.
		ranges := shardRanges(len(tokenized), workers)
		shards := make([]anchorShard, len(ranges))
		var wg sync.WaitGroup
		for s, r := range ranges {
			wg.Add(1)
			go func(s, lo, hi int) {
				defer wg.Done()
				scratch := newAnchorScratch(numIDs)
				canonical := &canonicalScratch{}
				shard := &shards[s]
				shard.lo, shard.hi = lo, hi
				shard.keyOff = append(shard.keyOff, 0)
				shard.ancOff = append(shard.ancOff, 0)
				for row := lo; row < hi; row++ {
					anchors := anchorTokens(tokenized[row], rowIDs[row], idep, threshold, scratch)
					keyBytes, ordered := canonicalAnchorKey(anchors, canonical)
					shard.keys = append(shard.keys, keyBytes...)
					shard.ancs = append(shard.ancs, ordered...)
					shard.keyOff = append(shard.keyOff, len(shard.keys))
					shard.ancOff = append(shard.ancOff, len(shard.ancs))
				}
			}(s, r[0], r[1])
		}
		wg.Wait()
		for _, shard := range shards {
			for row := shard.lo; row < shard.hi; row++ {
				j := row - shard.lo
				keyBytes := shard.keys[shard.keyOff[j]:shard.keyOff[j+1]]
				ordered := shard.ancs[shard.ancOff[j]:shard.ancOff[j+1]]
				addRow(row, keyBytes, ordered)
			}
		}
	} else {
		scratch := newAnchorScratch(numIDs)
		canonical := &canonicalScratch{}
		for row := range tokenized {
			anchors := anchorTokens(tokenized[row], rowIDs[row], idep, threshold, scratch)
			keyBytes, ordered := canonicalAnchorKey(anchors, canonical)
			addRow(row, keyBytes, ordered)
		}
	}

	// Message indices land ascending because rowOf is iterated in order.
	for idx, row := range rowOf {
		rowGroup[row].indices = append(rowGroup[row].indices, idx)
	}

	groups := make([]anchorGroup, 0, len(merged))
	for _, group := range merged {
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

// tokenizeMessagesParallelInto is tokenizeMessagesInto sharded over workers.
// Rows are written to disjoint slots, so output is identical to the serial
// version. Each worker owns its scratch.
func tokenizeMessagesParallelInto(
	messages []string,
	tokenizer *Tokenizer,
	dst [][]Token,
	workers int,
) [][]Token {
	tokenized := dst
	if cap(tokenized) < len(messages) {
		tokenized = make([][]Token, len(messages))
	} else {
		tokenized = tokenized[:len(messages)]
	}
	var wg sync.WaitGroup
	for _, r := range shardRanges(len(messages), workers) {
		wg.Add(1)
		go func(lo, hi int) {
			defer wg.Done()
			var scratch tokenizationScratch
			for i := lo; i < hi; i++ {
				tokenized[i] = tokenizer.TokenizeInto(messages[i], tokenized[i], &scratch)
			}
		}(r[0], r[1])
	}
	wg.Wait()
	return tokenized
}

// shardRanges yields contiguous [lo,hi) ranges covering n rows across at most
// workers shards.
func shardRanges(n, workers int) [][2]int {
	chunk := (n + workers - 1) / workers
	out := make([][2]int, 0, workers)
	for lo := 0; lo < n; lo += chunk {
		hi := lo + chunk
		if hi > n {
			hi = n
		}
		out = append(out, [2]int{lo, hi})
	}
	return out
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

func retokenizeMessagesWithSymbolsInto(
	tokenized [][]Token,
	symbols *symbolTable,
	dst [][]Token,
	workers int,
) [][]Token {
	out := dst
	if cap(out) < len(tokenized) {
		out = make([][]Token, len(tokenized))
	} else {
		out = out[:len(tokenized)]
	}
	if workers > 1 {
		var wg sync.WaitGroup
		for _, r := range shardRanges(len(tokenized), workers) {
			wg.Add(1)
			go func(lo, hi int) {
				defer wg.Done()
				for i := lo; i < hi; i++ {
					out[i] = retokenizeTokensWithSymbolsInto(tokenized[i], symbols, out[i])
				}
			}(r[0], r[1])
		}
		wg.Wait()
		return out
	}
	for i := range tokenized {
		out[i] = retokenizeTokensWithSymbolsInto(tokenized[i], symbols, out[i])
	}
	return out
}

func retokenizeTokensWithSymbolsInto(tokens []Token, symbols *symbolTable, dst []Token) []Token {
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

func ensureInt32Slice(buf []int32, n int) []int32 {
	if cap(buf) < n {
		return make([]int32, n)
	}
	return buf[:n]
}

func ensureStringSlice(buf []string, n int) []string {
	if cap(buf) < n {
		return make([]string, n)
	}
	return buf[:n]
}

func tokenNeedsSplit(slice string, symbols *symbolTable) bool {
	for i := 0; i < len(slice); {
		if b := slice[i]; b < utf8.RuneSelf {
			if symbols.class[b] == classSymbol {
				return true
			}
			i++
			continue
		}
		r, size := utf8.DecodeRuneInString(slice[i:])
		if symbols.isSymbolRune(r) {
			return true
		}
		i += size
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

// canonicalAnchorKey returns the canonical key bytes for the anchor set and
// the anchors ordered by key. Both are backed by scratch and only valid until
// the next call.
func canonicalAnchorKey(anchors map[tokenKey]Token, scratch *canonicalScratch) ([]byte, []Token) {
	scratch.keyBuf = scratch.keyBuf[:0]
	if len(anchors) == 0 {
		scratch.keys = scratch.keys[:0]
		scratch.ordered = scratch.ordered[:0]
		return scratch.keyBuf, scratch.ordered
	}
	keys := scratch.keys[:0]
	for k := range anchors {
		keys = append(keys, k)
	}
	slices.SortFunc(keys, func(a, b tokenKey) int {
		if a.kind != b.kind {
			return int(a.kind) - int(b.kind)
		}
		return cmp.Compare(a.slice, b.slice)
	})
	scratch.keys = keys

	ordered := scratch.ordered
	if cap(ordered) < len(keys) {
		ordered = make([]Token, len(keys))
	} else {
		ordered = ordered[:len(keys)]
	}

	buf := scratch.keyBuf
	for i, k := range keys {
		ordered[i] = anchors[k]
		buf = append(buf, byte(k.kind), '\x1f')
		buf = append(buf, k.slice...)
		buf = append(buf, '\x00')
	}
	scratch.keyBuf = buf
	scratch.ordered = ordered
	return buf, ordered
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
