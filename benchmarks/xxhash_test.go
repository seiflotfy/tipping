package benchmarks

import (
	"testing"

	"github.com/go-simd/xxhash"
)

// These benchmarks answer "should tipping front its string-keyed maps (line
// dedup, anchor-group keys) with go-simd/xxhash?" by comparing Go's built-in
// string map (hardware AES hashing inside the runtime) against XXH3-64 +
// a uint64-keyed map on the same workload.

func BenchmarkDedupStringMap(b *testing.B) {
	lines := corpusDup4K
	m := make(map[string]int32, len(lines))
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		clear(m)
		for _, l := range lines {
			if _, ok := m[l]; !ok {
				m[l] = int32(len(m))
			}
		}
		benchSink = len(m)
	}
}

func BenchmarkDedupXXH3Map(b *testing.B) {
	lines := corpusDup4K
	m := make(map[uint64]int32, len(lines))
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		clear(m)
		for _, l := range lines {
			h := xxhash.Sum64String(l)
			if _, ok := m[h]; !ok {
				m[h] = int32(len(m))
			}
		}
		benchSink = len(m)
	}
}
