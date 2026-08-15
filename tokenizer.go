package tipping

import (
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"
)

// TokenKind classifies a token extracted from a log line.
type TokenKind uint8

const (
	TokenAlphabetic TokenKind = iota
	TokenNumeric
	TokenSymbolic
	TokenWhitespace
	TokenImpure
	TokenSpecialWhite
	TokenSpecialBlack
)

// Token is the typed slice used by the parser pipeline.
type Token struct {
	Kind  TokenKind
	Slice string
}

type tokenKey struct {
	kind  TokenKind
	slice string
}

type preTokenKind uint8

const (
	preTokenUnrefined preTokenKind = iota
	preTokenSpecialWhite
	preTokenSpecialBlack
)

type preToken struct {
	kind  preTokenKind
	slice string
}

type specialMatcher struct {
	literal string
	re      *regexp.Regexp
}

const (
	classOther uint8 = iota
	classSpace
	classSymbol
)

// symbolTable is a precomputed byte-class table for ASCII plus a rune-set
// fallback for non-ASCII symbols. It answers "is this rune a split symbol or
// whitespace" without map lookups on the ASCII hot path. symLo is the same
// ASCII symbol set encoded shufti-style for the SIMD kernel: bit (b>>4) of
// symLo[b&0xF] is set iff byte b is a split symbol.
type symbolTable struct {
	class    [128]uint8
	symLo    [16]byte
	nonASCII map[rune]struct{}
}

func newSymbolTable(symbols map[rune]struct{}) *symbolTable {
	t := &symbolTable{}
	for b := 0; b < 128; b++ {
		if asciiSpace(byte(b)) {
			t.class[b] = classSpace
		}
	}
	for r := range symbols {
		if r < 128 {
			if t.class[r] == classOther {
				t.class[r] = classSymbol
			}
			t.symLo[r&0xF] |= 1 << (r >> 4)
			continue
		}
		if t.nonASCII == nil {
			t.nonASCII = make(map[rune]struct{})
		}
		t.nonASCII[r] = struct{}{}
	}
	return t
}

func asciiSpace(b byte) bool {
	return b == ' ' || b == '\t' || b == '\n' || b == '\v' || b == '\f' || b == '\r'
}

func (t *symbolTable) isSymbolRune(r rune) bool {
	if r < 128 {
		return t.class[r] == classSymbol
	}
	_, ok := t.nonASCII[r]
	return ok
}

// Tokenizer applies special regex tokenization followed by symbol/whitespace splitting.
type Tokenizer struct {
	specialWhites []specialMatcher
	specialBlacks []specialMatcher
	symbols       *symbolTable
}

type tokenizationScratch struct {
	pre  []preToken
	next []preToken
}

func newSymbolSet(symbols string) map[rune]struct{} {
	if symbols == "" {
		return map[rune]struct{}{}
	}
	set := make(map[rune]struct{}, utf8.RuneCountInString(symbols))
	for _, r := range symbols {
		set[r] = struct{}{}
	}
	return set
}

func cloneSymbolSet(symbols map[rune]struct{}) map[rune]struct{} {
	out := make(map[rune]struct{}, len(symbols))
	for r := range symbols {
		out[r] = struct{}{}
	}
	return out
}

// NewTokenizer constructs a tokenizer.
func NewTokenizer(
	specialWhites []*regexp.Regexp,
	specialBlacks []*regexp.Regexp,
	symbols map[rune]struct{},
) *Tokenizer {
	return &Tokenizer{
		specialWhites: compileSpecialMatchers(specialWhites),
		specialBlacks: compileSpecialMatchers(specialBlacks),
		symbols:       newSymbolTable(symbols),
	}
}

func (t *Tokenizer) cloneWithSymbols(symbols *symbolTable) *Tokenizer {
	return &Tokenizer{
		specialWhites: append([]specialMatcher(nil), t.specialWhites...),
		specialBlacks: append([]specialMatcher(nil), t.specialBlacks...),
		symbols:       symbols,
	}
}

// Tokenize splits a log message into typed tokens.
func (t *Tokenizer) Tokenize(msg string) []Token {
	return t.TokenizeInto(msg, nil, nil)
}

func (t *Tokenizer) TokenizeInto(msg string, dst []Token, scratch *tokenizationScratch) []Token {
	pre := t.preTokenizeInto(msg, scratch)
	out := dst[:0]
	if len(pre) == 1 {
		switch pre[0].kind {
		case preTokenSpecialWhite:
			return append(out, Token{Kind: TokenSpecialWhite, Slice: pre[0].slice})
		case preTokenSpecialBlack:
			return append(out, Token{Kind: TokenSpecialBlack, Slice: pre[0].slice})
		default:
			if cap(out) == 0 {
				out = make([]Token, 0, len(pre[0].slice)/2+1)
			}
			return appendSplitToken(out, pre[0].slice, t.symbols)
		}
	}

	if cap(out) == 0 {
		out = make([]Token, 0, len(pre)*2)
	}
	for _, p := range pre {
		switch p.kind {
		case preTokenSpecialWhite:
			out = append(out, Token{Kind: TokenSpecialWhite, Slice: p.slice})
		case preTokenSpecialBlack:
			out = append(out, Token{Kind: TokenSpecialBlack, Slice: p.slice})
		default:
			out = appendSplitToken(out, p.slice, t.symbols)
		}
	}
	return out
}

func (t *Tokenizer) preTokenize(msg string) []preToken {
	return t.preTokenizeInto(msg, nil)
}

func (t *Tokenizer) preTokenizeInto(msg string, scratch *tokenizationScratch) []preToken {
	if scratch == nil {
		preTokens := []preToken{{kind: preTokenUnrefined, slice: msg}}

		for _, matcher := range t.specialWhites {
			next := make([]preToken, 0, len(preTokens)*2)
			for _, p := range preTokens {
				switch p.kind {
				case preTokenSpecialWhite, preTokenSpecialBlack:
					next = append(next, p)
				default:
					next = appendSplitSpecial(next, p.slice, matcher, preTokenSpecialWhite)
				}
			}
			preTokens = next
		}

		for _, matcher := range t.specialBlacks {
			next := make([]preToken, 0, len(preTokens)*2)
			for _, p := range preTokens {
				switch p.kind {
				case preTokenSpecialWhite, preTokenSpecialBlack:
					next = append(next, p)
				default:
					next = appendSplitSpecial(next, p.slice, matcher, preTokenSpecialBlack)
				}
			}
			preTokens = next
		}

		return preTokens
	}

	preTokens := scratch.pre[:0]
	preTokens = append(preTokens, preToken{kind: preTokenUnrefined, slice: msg})
	next := scratch.next[:0]

	for _, matcher := range t.specialWhites {
		next = next[:0]
		for _, p := range preTokens {
			switch p.kind {
			case preTokenSpecialWhite, preTokenSpecialBlack:
				next = append(next, p)
			default:
				next = appendSplitSpecial(next, p.slice, matcher, preTokenSpecialWhite)
			}
		}
		preTokens, next = next, preTokens
	}

	for _, matcher := range t.specialBlacks {
		next = next[:0]
		for _, p := range preTokens {
			switch p.kind {
			case preTokenSpecialWhite, preTokenSpecialBlack:
				next = append(next, p)
			default:
				next = appendSplitSpecial(next, p.slice, matcher, preTokenSpecialBlack)
			}
		}
		preTokens, next = next, preTokens
	}

	scratch.pre = preTokens
	scratch.next = next
	return preTokens
}

func compileSpecialMatchers(regexes []*regexp.Regexp) []specialMatcher {
	out := make([]specialMatcher, 0, len(regexes))
	for _, re := range regexes {
		pattern := re.String()
		if literal, ok := literalPattern(pattern); ok {
			out = append(out, specialMatcher{literal: literal})
			continue
		}
		out = append(out, specialMatcher{re: re})
	}
	return out
}

func literalPattern(pattern string) (string, bool) {
	if pattern == "" {
		return "", false
	}
	if regexp.QuoteMeta(pattern) != pattern {
		return "", false
	}
	return pattern, true
}

func appendSplitSpecial(dst []preToken, msg string, matcher specialMatcher, kind preTokenKind) []preToken {
	if matcher.literal != "" {
		return appendSplitSpecialLiteral(dst, msg, matcher.literal, kind)
	}
	return appendSplitSpecialRegex(dst, msg, matcher.re, kind)
}

func appendSplitSpecialRegex(dst []preToken, msg string, re *regexp.Regexp, kind preTokenKind) []preToken {
	if re == nil {
		return append(dst, preToken{kind: preTokenUnrefined, slice: msg})
	}
	last := 0
	matched := false
	for last <= len(msg) {
		idx := re.FindStringIndex(msg[last:])
		if idx == nil {
			break
		}
		start := last + idx[0]
		end := last + idx[1]
		if end-start <= 0 {
			if start >= len(msg) {
				break
			}
			_, size := utf8.DecodeRuneInString(msg[start:])
			if size <= 0 {
				size = 1
			}
			last = start + size
			continue
		}
		matched = true
		if start > last {
			dst = append(dst, preToken{kind: preTokenUnrefined, slice: msg[last:start]})
		}
		dst = append(dst, preToken{kind: kind, slice: msg[start:end]})
		last = end
	}
	if !matched {
		return append(dst, preToken{kind: preTokenUnrefined, slice: msg})
	}
	if last < len(msg) {
		dst = append(dst, preToken{kind: preTokenUnrefined, slice: msg[last:]})
	}
	return dst
}

func appendSplitSpecialLiteral(dst []preToken, msg, literal string, kind preTokenKind) []preToken {
	if literal == "" {
		return append(dst, preToken{kind: preTokenUnrefined, slice: msg})
	}
	count := strings.Count(msg, literal)
	if count == 0 {
		return append(dst, preToken{kind: preTokenUnrefined, slice: msg})
	}

	last := 0
	litLen := len(literal)
	for {
		next := strings.Index(msg[last:], literal)
		if next < 0 {
			break
		}
		start := last + next
		if start > last {
			dst = append(dst, preToken{kind: preTokenUnrefined, slice: msg[last:start]})
		}
		end := start + litLen
		dst = append(dst, preToken{kind: kind, slice: msg[start:end]})
		last = end
	}
	if last < len(msg) {
		dst = append(dst, preToken{kind: preTokenUnrefined, slice: msg[last:]})
	}
	return dst
}

func splitToken(msg string, symbols *symbolTable) []Token {
	if msg == "" {
		return nil
	}
	tokens := make([]Token, 0, splitTokenCount(msg, symbols))
	return appendSplitToken(tokens, msg, symbols)
}

func appendSplitToken(tokens []Token, msg string, symbols *symbolTable) []Token {
	if simdTokenize && len(msg) >= simdMinLen && len(msg) <= simdMaxLen {
		if out, ok := appendSplitTokenSIMD(tokens, msg, symbols); ok {
			return out
		}
	}
	return appendSplitTokenScalar(tokens, msg, symbols)
}

func appendSplitTokenScalar(tokens []Token, msg string, symbols *symbolTable) []Token {
	if msg == "" {
		return tokens
	}
	start := 0
	i := 0
	for i < len(msg) {
		if b := msg[i]; b < utf8.RuneSelf {
			cls := symbols.class[b]
			if cls == classOther {
				i++
				continue
			}
			if start < i {
				tokens = append(tokens, tokenWith(msg[start:i], symbols))
			}
			kind := TokenSymbolic
			if cls == classSpace {
				kind = TokenWhitespace
			}
			tokens = append(tokens, Token{Kind: kind, Slice: msg[i : i+1]})
			i++
			start = i
			continue
		}
		r, size := utf8.DecodeRuneInString(msg[i:])
		isSpace := unicode.IsSpace(r)
		if !isSpace && !symbols.isSymbolRune(r) {
			i += size
			continue
		}
		if start < i {
			tokens = append(tokens, tokenWith(msg[start:i], symbols))
		}
		kind := TokenSymbolic
		if isSpace {
			kind = TokenWhitespace
		}
		tokens = append(tokens, Token{Kind: kind, Slice: msg[i : i+size]})
		i += size
		start = i
	}

	if start < len(msg) {
		tokens = append(tokens, tokenWith(msg[start:], symbols))
	}

	return tokens
}

func splitTokenCount(msg string, symbols *symbolTable) int {
	if msg == "" {
		return 0
	}
	count := 0
	start := 0
	i := 0
	for i < len(msg) {
		if b := msg[i]; b < utf8.RuneSelf {
			if symbols.class[b] == classOther {
				i++
				continue
			}
			if start < i {
				count++
			}
			count++
			i++
			start = i
			continue
		}
		r, size := utf8.DecodeRuneInString(msg[i:])
		if !unicode.IsSpace(r) && !symbols.isSymbolRune(r) {
			i += size
			continue
		}
		if start < i {
			count++
		}
		count++
		i += size
		start = i
	}
	if start < len(msg) {
		count++
	}
	return count
}

func tokenWith(slice string, symbols *symbolTable) Token {
	// ASCII fast path: classify with byte checks only.
	allLetter := true
	allDigit := true
	for i := 0; i < len(slice); i++ {
		b := slice[i]
		if b >= utf8.RuneSelf {
			return tokenWithSlow(slice, symbols)
		}
		if lower := b | 0x20; lower < 'a' || lower > 'z' {
			allLetter = false
		}
		if b < '0' || b > '9' {
			allDigit = false
		}
	}
	if len(slice) > 0 {
		if allLetter {
			return Token{Kind: TokenAlphabetic, Slice: slice}
		}
		if allDigit {
			return Token{Kind: TokenNumeric, Slice: slice}
		}
		if len(slice) == 1 {
			switch symbols.class[slice[0]] {
			case classSpace:
				return Token{Kind: TokenWhitespace, Slice: slice}
			case classSymbol:
				return Token{Kind: TokenSymbolic, Slice: slice}
			}
		}
	}
	return Token{Kind: TokenImpure, Slice: slice}
}

func tokenWithSlow(slice string, symbols *symbolTable) Token {
	if isAll(slice, unicode.IsLetter) {
		return Token{Kind: TokenAlphabetic, Slice: slice}
	}
	if isAll(slice, unicode.IsDigit) {
		return Token{Kind: TokenNumeric, Slice: slice}
	}

	r, size := utf8.DecodeRuneInString(slice)
	if size == len(slice) {
		if unicode.IsSpace(r) {
			return Token{Kind: TokenWhitespace, Slice: slice}
		}
		if symbols.isSymbolRune(r) {
			return Token{Kind: TokenSymbolic, Slice: slice}
		}
	}
	return Token{Kind: TokenImpure, Slice: slice}
}

func isAll(s string, predicate func(rune) bool) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if !predicate(r) {
			return false
		}
	}
	return true
}

func tokenKeyFor(tok Token) tokenKey {
	return tokenKey{kind: tok.Kind, slice: tok.Slice}
}
