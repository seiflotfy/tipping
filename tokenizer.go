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

// Tokenizer applies special regex tokenization followed by symbol/whitespace splitting.
type Tokenizer struct {
	specialWhites []specialMatcher
	specialBlacks []specialMatcher
	symbols       map[rune]struct{}
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
		symbols:       cloneSymbolSet(symbols),
	}
}

func (t *Tokenizer) cloneWithSymbols(symbols map[rune]struct{}) *Tokenizer {
	return &Tokenizer{
		specialWhites: append([]specialMatcher(nil), t.specialWhites...),
		specialBlacks: append([]specialMatcher(nil), t.specialBlacks...),
		symbols:       cloneSymbolSet(symbols),
	}
}

// Tokenize splits a log message into typed tokens.
func (t *Tokenizer) Tokenize(msg string) []Token {
	pre := t.preTokenize(msg)
	tokens := make([]Token, 0, len(pre)*2)
	for _, p := range pre {
		switch p.kind {
		case preTokenSpecialWhite:
			tokens = append(tokens, Token{Kind: TokenSpecialWhite, Slice: p.slice})
		case preTokenSpecialBlack:
			tokens = append(tokens, Token{Kind: TokenSpecialBlack, Slice: p.slice})
		default:
			tokens = append(tokens, splitToken(p.slice, t.symbols)...)
		}
	}
	return tokens
}

func (t *Tokenizer) preTokenize(msg string) []preToken {
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

func splitToken(msg string, symbols map[rune]struct{}) []Token {
	if msg == "" {
		return nil
	}

	tokens := make([]Token, 0, splitTokenCount(msg, symbols))
	start := 0
	for i, r := range msg {
		_, isSymbol := symbols[r]
		if !unicode.IsSpace(r) && !isSymbol {
			continue
		}
		if start < i {
			tokens = append(tokens, tokenWith(msg[start:i], symbols))
		}
		size := utf8.RuneLen(r)
		if size < 0 {
			size = 1
		}
		kind := TokenSymbolic
		if unicode.IsSpace(r) {
			kind = TokenWhitespace
		}
		tokens = append(tokens, Token{Kind: kind, Slice: msg[i : i+size]})
		start = i + size
	}

	if start < len(msg) {
		tokens = append(tokens, tokenWith(msg[start:], symbols))
	}

	return tokens
}

func splitTokenCount(msg string, symbols map[rune]struct{}) int {
	if msg == "" {
		return 0
	}
	count := 0
	start := 0
	for i, r := range msg {
		_, isSymbol := symbols[r]
		if !unicode.IsSpace(r) && !isSymbol {
			continue
		}
		if start < i {
			count++
		}
		count++
		size := utf8.RuneLen(r)
		if size < 0 {
			size = 1
		}
		start = i + size
	}
	if start < len(msg) {
		count++
	}
	return count
}

func tokenWith(slice string, symbols map[rune]struct{}) Token {
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
		if _, ok := symbols[r]; ok {
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
