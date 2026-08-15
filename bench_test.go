package tipping

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

var (
	benchClusters  []int
	benchTemplates [][]string
	benchMasks     []string
	benchTokens    []Token
)

var (
	benchDefaultCorpus = mustLoadCorpus("default.log")
	benchSpecialCorpus = mustLoadCorpus("special.log")
	benchDefault4K     = expandToAtLeast(benchDefaultCorpus, 4096)
	benchSpecial4K     = expandToAtLeast(benchSpecialCorpus, 4096)
	benchUnique4K      = makeUniqueCorpus(4096)
)

// makeUniqueCorpus generates n distinct lines across 4 shapes: the worst case
// for line deduplication (every line unique).
func makeUniqueCorpus(n int) []string {
	out := make([]string, 0, n)
	for i := 0; i < n; i++ {
		switch i % 4 {
		case 0:
			out = append(out, fmt.Sprintf("Accepted password for user%d from 10.0.%d.%d port %d ssh2", i, i%256, i%199, 40000+i))
		case 1:
			out = append(out, fmt.Sprintf("Connection closed by 192.168.%d.%d [preauth]", i%256, i%97))
		case 2:
			out = append(out, fmt.Sprintf("block blk_%d received exception java.io.IOException", 100000+i))
		default:
			out = append(out, fmt.Sprintf("PacketResponder %d for block blk_%d terminating", i%3, 200000+i))
		}
	}
	return out
}

func BenchmarkParseUnique4KReuseBuffers(b *testing.B) {
	p := NewParser()
	p.WithFilterAlphabetic(true)
	bufs := NewParseBuffers()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		benchClusters = p.ParseInto(benchUnique4K, bufs)
	}
}

func BenchmarkParseWithTemplatesAndMasksUnique4KReuseBuffers(b *testing.B) {
	p := NewParser()
	p.WithFilterAlphabetic(true)
	bufs := NewParseBuffers()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		benchClusters, benchTemplates, benchMasks = p.ParseWithTemplatesAndMasksInto(benchUnique4K, bufs)
	}
}

func BenchmarkParseDefault4K(b *testing.B) {
	p := NewParser()
	p.WithFilterAlphabetic(true)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		benchClusters = p.Parse(benchDefault4K)
	}
}

func BenchmarkParseDefault4KReuseBuffers(b *testing.B) {
	p := NewParser()
	p.WithFilterAlphabetic(true)
	bufs := NewParseBuffers()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		benchClusters = p.ParseInto(benchDefault4K, bufs)
	}
}

func BenchmarkParseWithTemplatesAndMasksDefault4K(b *testing.B) {
	p := NewParser()
	p.WithFilterAlphabetic(true)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		benchClusters, benchTemplates, benchMasks = p.ParseWithTemplatesAndMasks(benchDefault4K)
	}
}

func BenchmarkParseWithTemplatesAndMasksDefault4KReuseBuffers(b *testing.B) {
	p := NewParser()
	p.WithFilterAlphabetic(true)
	bufs := NewParseBuffers()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		benchClusters, benchTemplates, benchMasks = p.ParseWithTemplatesAndMasksInto(benchDefault4K, bufs)
	}
}

func BenchmarkParseWithTemplatesAndMasksSpecial4K(b *testing.B) {
	white, err := CompilePatterns([]string{`Fan`, `Temp`})
	if err != nil {
		b.Fatalf("compile special whites: %v", err)
	}
	black, err := CompilePatterns([]string{`\d+\.\d+`})
	if err != nil {
		b.Fatalf("compile special blacks: %v", err)
	}

	p := NewParser().
		WithSymbols(".").
		WithFilterAlphabetic(true).
		WithSpecialWhites(white).
		WithSpecialBlacks(black)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		benchClusters, benchTemplates, benchMasks = p.ParseWithTemplatesAndMasks(benchSpecial4K)
	}
}

func BenchmarkParseWithTemplatesAndMasksSpecial4KReuseBuffers(b *testing.B) {
	white, err := CompilePatterns([]string{`Fan`, `Temp`})
	if err != nil {
		b.Fatalf("compile special whites: %v", err)
	}
	black, err := CompilePatterns([]string{`\d+\.\d+`})
	if err != nil {
		b.Fatalf("compile special blacks: %v", err)
	}

	p := NewParser().
		WithSymbols(".").
		WithFilterAlphabetic(true).
		WithSpecialWhites(white).
		WithSpecialBlacks(black)
	bufs := NewParseBuffers()

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		benchClusters, benchTemplates, benchMasks = p.ParseWithTemplatesAndMasksInto(benchSpecial4K, bufs)
	}
}

func BenchmarkTokenizerDefaultLine(b *testing.B) {
	tok := NewTokenizer(nil, nil, map[rune]struct{}{})
	msg := "a x1 x2 b"
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		benchTokens = tok.Tokenize(msg)
	}
}

func BenchmarkTokenizerRealisticLine(b *testing.B) {
	tok := NewTokenizer(nil, nil, newSymbolSet(DefaultSymbols))
	msg := "2024-03-01T12:34:56.789Z INFO sshd[24206]: Accepted password for user42 from 10.0.3.117 port 40122 ssh2 (session=af31bc)"
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		benchTokens = tok.Tokenize(msg)
	}
}

func BenchmarkTokenizerSpecialLine(b *testing.B) {
	white := []*regexp.Regexp{regexp.MustCompile(`fan_\d+`)}
	black := []*regexp.Regexp{regexp.MustCompile(`\d+\.\d+`)}
	tok := NewTokenizer(white, black, newSymbolSet("."))
	msg := "Fan fan_2 speed is set to 12.3114 on machine sys.node.fan_3 on node 12"
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		benchTokens = tok.Tokenize(msg)
	}
}

func mustLoadCorpus(file string) []string {
	path := filepath.Join("testdata", "corpus", file)
	data, err := os.ReadFile(path)
	if err != nil {
		panic(err)
	}
	raw := strings.Split(strings.ReplaceAll(string(data), "\r\n", "\n"), "\n")
	lines := make([]string, 0, len(raw))
	for _, line := range raw {
		if line == "" {
			continue
		}
		lines = append(lines, line)
	}
	if len(lines) == 0 {
		panic("empty corpus: " + file)
	}
	return lines
}

func expandToAtLeast(lines []string, target int) []string {
	if len(lines) >= target {
		return append([]string(nil), lines...)
	}
	out := make([]string, 0, target)
	for len(out) < target {
		out = append(out, lines...)
	}
	return out[:target]
}
