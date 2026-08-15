//go:build (!arm64 && !amd64) || purego

package tipping

// Without a vector kernel the scalar tokenizer is at least as fast as
// emulating the class pass, so the SIMD path stays off.
const simdTokenize = false

func classifyBytes(p *byte, n int, symLo *[16]byte, out *byte) {
	classifyBytesGo(p, n, symLo, out)
}
