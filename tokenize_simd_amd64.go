//go:build amd64 && !purego

package tipping

// The kernel needs PSHUFB (SSSE3) for the shufti symbol lookup; everything
// else is SSE2. Detected once at init.
var simdTokenize = cpuHasSSSE3()

// classifyBytes writes one class byte per input byte for the n bytes at p.
// n must be >= 16: the trailing partial chunk is handled with an overlapping
// 16-byte load. Implemented in tokenize_simd_amd64.s; must match
// classifyBytesGo exactly. Only called when simdTokenize is true.
//
//go:noescape
func classifyBytes(p *byte, n int, symLo *[16]byte, out *byte)

func cpuHasSSSE3() bool
