package tipping

import "unsafe"

// The SIMD tokenizer classifies every byte of a line in one vector pass,
// writing a class byte per input byte. The consuming loop then emits tokens
// with a single scan and no per-byte table lookups: segment kinds
// (alphabetic/numeric/impure) fall out of AND-accumulating the class bits
// between boundaries, replacing tokenWith's re-scan of each segment.
const (
	classSpaceBit    = 1 << 0
	classSymbolBit   = 1 << 1
	classLetterBit   = 1 << 2
	classDigitBit    = 1 << 3
	classNonASCIIBit = 1 << 4

	classBoundaryBits = classSpaceBit | classSymbolBit | classNonASCIIBit

	// simdMinLen is the measured break-even: below ~32 bytes the kernel-call
	// and setup overhead beats the vector win. simdMaxLen bounds the stack
	// class buffer.
	// ponytail: lines over 1KiB take the scalar path; chunk the buffer if
	// long-line corpora ever matter.
	simdMinLen = 32
	simdMaxLen = 1024
)

// appendSplitTokenSIMD is the vector-classified equivalent of
// appendSplitTokenScalar for ASCII input. ok=false means the line contains
// non-ASCII bytes (which need unicode-aware splitting) and nothing was
// appended; the caller retries with the scalar path.
func appendSplitTokenSIMD(tokens []Token, msg string, symbols *symbolTable) ([]Token, bool) {
	var classes [simdMaxLen]byte
	n := len(msg)
	classifyBytes(unsafe.StringData(msg), n, &symbols.symLo, &classes[0])

	origLen := len(tokens)
	start := 0
	acc := byte(0xFF)
	for i := 0; i < n; i++ {
		c := classes[i]
		if c&classBoundaryBits == 0 {
			acc &= c
			continue
		}
		if c&classNonASCIIBit != 0 {
			return tokens[:origLen], false
		}
		if start < i {
			tokens = append(tokens, segmentToken(msg[start:i], acc))
		}
		kind := TokenSymbolic
		if c&classSpaceBit != 0 {
			kind = TokenWhitespace
		}
		tokens = append(tokens, Token{Kind: kind, Slice: msg[i : i+1]})
		start = i + 1
		acc = 0xFF
	}
	if start < n {
		tokens = append(tokens, segmentToken(msg[start:], acc))
	}
	return tokens, true
}

// segmentToken classifies a boundary-free segment from its accumulated class
// bits. Space/symbol kinds are impossible here: those bytes are boundaries.
func segmentToken(slice string, acc byte) Token {
	if acc&classLetterBit != 0 {
		return Token{Kind: TokenAlphabetic, Slice: slice}
	}
	if acc&classDigitBit != 0 {
		return Token{Kind: TokenNumeric, Slice: slice}
	}
	return Token{Kind: TokenImpure, Slice: slice}
}

// classifyBytesGo is the portable reference implementation of the kernel; the
// assembly must produce byte-identical output.
func classifyBytesGo(p *byte, n int, symLo *[16]byte, out *byte) {
	src := unsafe.Slice(p, n)
	dst := unsafe.Slice(out, n)
	for i, b := range src {
		var c byte
		if b == ' ' || (b >= '\t' && b <= '\r') {
			c |= classSpaceBit
		}
		if lower := b | 0x20; lower >= 'a' && lower <= 'z' {
			c |= classLetterBit
		}
		if b >= '0' && b <= '9' {
			c |= classDigitBit
		}
		if b >= 0x80 {
			c |= classNonASCIIBit
		}
		if b < 0x80 && symLo[b&0xF]&(1<<(b>>4)) != 0 {
			c |= classSymbolBit
		}
		dst[i] = c
	}
}
