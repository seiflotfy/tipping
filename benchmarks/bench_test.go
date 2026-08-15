// Package benchmarks compares tipping against github.com/axiomhq/drain3 on
// shared corpora. It lives in its own module so the tipping library keeps a
// dependency-free go.mod.
package benchmarks

import (
	"fmt"
	"testing"

	"github.com/axiomhq/drain3"
	"github.com/seiflotfy/tipping"
)

var (
	benchSink        int
	benchSinkMatcher *drain3.Matcher
)

// corpusDup4K: 4096 lines, ~200 unique — the common shape of production logs.
// corpusUnique4K: 4096 lines, all unique — the high-cardinality worst case.
var (
	corpusDup4K    = makeCorpus(4096, 50)
	corpusUnique4K = makeCorpus(4096, 1<<30)
)

// makeCorpus generates n lines across 4 templates; card bounds the parameter
// range, so smaller card means more duplicate lines.
func makeCorpus(n, card int) []string {
	out := make([]string, 0, n)
	for i := 0; i < n; i++ {
		v := i % card
		switch i % 4 {
		case 0:
			out = append(out, fmt.Sprintf("Accepted password for user%d from 10.0.%d.%d port %d ssh2", v, v%256, v%199, 40000+v))
		case 1:
			out = append(out, fmt.Sprintf("Connection closed by 192.168.%d.%d [preauth]", v%256, v%97))
		case 2:
			out = append(out, fmt.Sprintf("block blk_%d received exception java.io.IOException", 100000+v))
		default:
			out = append(out, fmt.Sprintf("PacketResponder %d for block blk_%d terminating", v%3, 200000+v))
		}
	}
	return out
}

func benchTippingParse(b *testing.B, corpus []string) {
	p := tipping.NewParser()
	bufs := tipping.NewParseBuffers()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		clusters := p.ParseInto(corpus, bufs)
		benchSink = len(clusters)
	}
}

func benchTippingParseWithTemplates(b *testing.B, corpus []string) {
	p := tipping.NewParser()
	bufs := tipping.NewParseBuffers()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		clusters, templates := p.ParseWithTemplatesInto(corpus, bufs)
		benchSink = len(clusters) + len(templates)
	}
}

func benchDrain3Train(b *testing.B, corpus []string) {
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		m, err := drain3.Train(corpus)
		if err != nil {
			b.Fatal(err)
		}
		benchSinkMatcher = m
	}
}

func BenchmarkTippingParseDup4K(b *testing.B)    { benchTippingParse(b, corpusDup4K) }
func BenchmarkTippingParseUnique4K(b *testing.B) { benchTippingParse(b, corpusUnique4K) }
func BenchmarkTippingParseWithTemplatesDup4K(b *testing.B) {
	benchTippingParseWithTemplates(b, corpusDup4K)
}
func BenchmarkTippingParseWithTemplatesUnique4K(b *testing.B) {
	benchTippingParseWithTemplates(b, corpusUnique4K)
}
func BenchmarkDrain3TrainDup4K(b *testing.B)    { benchDrain3Train(b, corpusDup4K) }
func BenchmarkDrain3TrainUnique4K(b *testing.B) { benchDrain3Train(b, corpusUnique4K) }

func BenchmarkTippingMatchID(b *testing.B) {
	p := tipping.NewParser()
	_, templates := p.ParseWithTemplates(corpusDup4K)
	m := tipping.NewMatcherFromTemplateSets(templates)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		id, _ := m.MatchID(corpusDup4K[i%len(corpusDup4K)])
		benchSink = id
	}
}

func BenchmarkDrain3MatchID(b *testing.B) {
	m, err := drain3.Train(corpusDup4K)
	if err != nil {
		b.Fatal(err)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		id, _ := m.MatchID(corpusDup4K[i%len(corpusDup4K)])
		benchSink = id
	}
}

// TestTemplateCounts prints what each miner learned, as a sanity check that
// the benchmarks compare comparable work (run with -v).
func TestTemplateCounts(t *testing.T) {
	for _, tc := range []struct {
		name   string
		corpus []string
	}{
		{"dup4k", corpusDup4K},
		{"unique4k", corpusUnique4K},
	} {
		p := tipping.NewParser()
		_, templates := p.ParseWithTemplates(tc.corpus)
		m, err := drain3.Train(tc.corpus)
		if err != nil {
			t.Fatal(err)
		}
		t.Logf("%s: tipping clusters=%d drain3 templates=%d", tc.name, len(templates), len(m.Templates()))
	}
}
