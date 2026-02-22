package tipping

import (
	"regexp"
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

// Tokenizer applies special regex tokenization followed by symbol/whitespace splitting.
type Tokenizer struct {
	specialWhites []*regexp.Regexp
	specialBlacks []*regexp.Regexp
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
	whiteCopy := append([]*regexp.Regexp(nil), specialWhites...)
	blackCopy := append([]*regexp.Regexp(nil), specialBlacks...)
	return &Tokenizer{
		specialWhites: whiteCopy,
		specialBlacks: blackCopy,
		symbols:       cloneSymbolSet(symbols),
	}
}

func (t *Tokenizer) cloneWithSymbols(symbols map[rune]struct{}) *Tokenizer {
	return &Tokenizer{
		specialWhites: append([]*regexp.Regexp(nil), t.specialWhites...),
		specialBlacks: append([]*regexp.Regexp(nil), t.specialBlacks...),
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

	for _, re := range t.specialWhites {
		next := make([]preToken, 0, len(preTokens)*2)
		for _, p := range preTokens {
			switch p.kind {
			case preTokenSpecialWhite, preTokenSpecialBlack:
				next = append(next, p)
			default:
				next = append(next, splitSpecial(p.slice, re, preTokenSpecialWhite)...)
			}
		}
		preTokens = next
	}

	for _, re := range t.specialBlacks {
		next := make([]preToken, 0, len(preTokens)*2)
		for _, p := range preTokens {
			switch p.kind {
			case preTokenSpecialWhite, preTokenSpecialBlack:
				next = append(next, p)
			default:
				next = append(next, splitSpecial(p.slice, re, preTokenSpecialBlack)...)
			}
		}
		preTokens = next
	}

	return preTokens
}

func splitSpecial(msg string, re *regexp.Regexp, kind preTokenKind) []preToken {
	indices := re.FindAllStringIndex(msg, -1)
	if len(indices) == 0 {
		return []preToken{{kind: preTokenUnrefined, slice: msg}}
	}

	out := make([]preToken, 0, len(indices)*2+1)
	last := 0
	for _, idx := range indices {
		start, end := idx[0], idx[1]
		if end-start <= 0 {
			continue
		}
		if start != last {
			out = append(out, preToken{kind: preTokenUnrefined, slice: msg[last:start]})
		}
		out = append(out, preToken{kind: kind, slice: msg[start:end]})
		last = end
	}
	if last != len(msg) {
		out = append(out, preToken{kind: preTokenUnrefined, slice: msg[last:]})
	}
	if len(out) == 0 {
		return []preToken{{kind: preTokenUnrefined, slice: msg}}
	}
	return out
}

func splitToken(msg string, symbols map[rune]struct{}) []Token {
	if msg == "" {
		return nil
	}

	tokens := make([]Token, 0, len(msg))
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
		tokens = append(tokens, tokenWith(msg[i:i+size], symbols))
		start = i + size
	}

	if start < len(msg) {
		tokens = append(tokens, tokenWith(msg[start:], symbols))
	}

	return tokens
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

func tokenIdentity(tok Token) string {
	return string(rune('0'+tok.Kind)) + "\x1f" + tok.Slice
}
