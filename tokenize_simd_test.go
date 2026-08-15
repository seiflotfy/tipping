package tipping

import (
	"math/rand"
	"strings"
	"testing"
)

// TestClassifyChunksMatchesReference validates the platform kernel (NEON on
// arm64) against the portable reference on randomized inputs and symbol sets.
func TestClassifyChunksMatchesReference(t *testing.T) {
	if !simdTokenize {
		t.Skip("no SIMD kernel on this platform")
	}
	rng := rand.New(rand.NewSource(1))
	for trial := 0; trial < 500; trial++ {
		table := randomSymbolTable(rng)
		n := 16 + rng.Intn(120) // >= 16, including non-multiples of 16
		src := make([]byte, n)
		for i := range src {
			if rng.Intn(8) == 0 {
				src[i] = byte(rng.Intn(256)) // include non-ASCII
			} else {
				src[i] = byte(rng.Intn(128))
			}
		}
		got := make([]byte, n)
		want := make([]byte, n)
		classifyBytes(&src[0], n, &table.symLo, &got[0])
		classifyBytesGo(&src[0], n, &table.symLo, &want[0])
		for i := range got {
			if got[i] != want[i] {
				t.Fatalf("trial %d: byte %d (0x%02x): kernel class 0b%05b, reference 0b%05b",
					trial, i, src[i], got[i], want[i])
			}
		}
	}
}

// TestAppendSplitTokenSIMDMatchesScalar validates the full SIMD tokenize path
// (including tail handling and non-ASCII fallback) against the scalar path.
func TestAppendSplitTokenSIMDMatchesScalar(t *testing.T) {
	rng := rand.New(rand.NewSource(2))
	alphabet := []rune("abcXYZ0189_-.,()= \t{}|:/日é")
	for trial := 0; trial < 2000; trial++ {
		table := randomSymbolTable(rng)
		var b strings.Builder
		n := rng.Intn(200)
		for i := 0; i < n; i++ {
			b.WriteRune(alphabet[rng.Intn(len(alphabet))])
		}
		msg := b.String()

		want := appendSplitTokenScalar(nil, msg, table)
		got := appendSplitToken(nil, msg, table)
		if len(got) != len(want) {
			t.Fatalf("trial %d: %q: token count %d != %d\n got: %v\nwant: %v", trial, msg, len(got), len(want), got, want)
		}
		for i := range got {
			if got[i] != want[i] {
				t.Fatalf("trial %d: %q: token %d: %v != %v", trial, msg, i, got[i], want[i])
			}
		}
	}
}

func randomSymbolTable(rng *rand.Rand) *symbolTable {
	candidates := "()[]{}=,*.:;|/\\-_<>!\"#$%&'+?@^`~"
	set := map[rune]struct{}{}
	for _, r := range candidates {
		if rng.Intn(3) == 0 {
			set[r] = struct{}{}
		}
	}
	return newSymbolTable(set)
}
