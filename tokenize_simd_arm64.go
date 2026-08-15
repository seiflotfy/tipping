//go:build arm64 && !purego

package tipping

const simdTokenize = true

// classifyBytes writes one class byte per input byte for the n bytes at p.
// n must be >= 16: the trailing partial chunk is handled with an overlapping
// 16-byte load. Implemented in tokenize_simd_arm64.s (NEON); must match
// classifyBytesGo exactly.
//
//go:noescape
func classifyBytes(p *byte, n int, symLo *[16]byte, out *byte)
