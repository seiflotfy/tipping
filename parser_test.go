package tipping

import (
	"fmt"
	"math"
	"reflect"
	"testing"
)

func TestParserTrivial(t *testing.T) {
	msgs := []string{
		"a x1 x2 b",
		"a x2 b",
		"a x3 b",
		"a x4 b",
		"c x1 d",
		"c x2 d",
		"c x3 d",
		"c x4 d",
	}

	parser := NewParser()
	clusters, templates, masks := parser.ParseWithTemplatesAndMasks(msgs)

	if len(clusters) != len(msgs) {
		t.Fatalf("clusters length mismatch: got %d want %d", len(clusters), len(msgs))
	}

	left := make(map[int]struct{})
	for _, id := range clusters[:4] {
		if id < 0 {
			t.Fatalf("unexpected unclustered message in first half")
		}
		left[id] = struct{}{}
	}
	right := make(map[int]struct{})
	for _, id := range clusters[4:] {
		if id < 0 {
			t.Fatalf("unexpected unclustered message in second half")
		}
		right[id] = struct{}{}
	}
	if len(left) != 1 {
		t.Fatalf("expected one cluster in first half, got %d", len(left))
	}
	if len(right) != 1 {
		t.Fatalf("expected one cluster in second half, got %d", len(right))
	}
	if clusters[0] == clusters[4] {
		t.Fatalf("expected distinct cluster ids for first and second half")
	}

	expectedTemps := map[string]struct{}{
		"a <*> b":     {},
		"a <*> <*> b": {},
		"c <*> d":     {},
	}
	allTemps := make(map[string]struct{})
	for _, ts := range templates {
		for _, temp := range ts {
			allTemps[temp] = struct{}{}
		}
	}
	if !reflect.DeepEqual(allTemps, expectedTemps) {
		t.Fatalf("templates mismatch\n got: %#v\nwant: %#v", allTemps, expectedTemps)
	}

	expectedMasks := []string{
		"001101100",
		"001100",
		"001100",
		"001100",
		"001100",
		"001100",
		"001100",
		"001100",
	}
	if !reflect.DeepEqual(masks, expectedMasks) {
		t.Fatalf("masks mismatch\n got: %#v\nwant: %#v", masks, expectedMasks)
	}

	onlyClusters := parser.Parse(msgs)
	if !reflect.DeepEqual(onlyClusters, clusters) {
		t.Fatalf("Parse cluster output drifted")
	}
}

func TestParseWithMasksPreservesDuplicates(t *testing.T) {
	msgs := []string{
		"svc id=100 path=/a",
		"svc id=100 path=/a",
		"svc id=200 path=/b",
	}

	parser := NewParser()
	clusters, masks := parser.ParseWithMasks(msgs)
	if len(clusters) != len(msgs) {
		t.Fatalf("clusters length mismatch: got %d want %d", len(clusters), len(msgs))
	}
	if len(masks) != len(msgs) {
		t.Fatalf("masks length mismatch: got %d want %d", len(masks), len(msgs))
	}
	if masks[0] != masks[1] {
		t.Fatalf("expected duplicate messages to keep duplicate mask rows: %q vs %q", masks[0], masks[1])
	}
}

func TestParseIntoParity(t *testing.T) {
	msgs := []string{
		"a x1 x2 b",
		"a x2 b",
		"a x3 b",
		"a x4 b",
		"c x1 d",
		"c x2 d",
	}

	parser := NewParser()
	bufs := NewParseBuffers()

	wantClusters, wantTemplates, wantMasks := parser.ParseWithTemplatesAndMasks(msgs)
	gotClusters, gotTemplates, gotMasks := parser.ParseWithTemplatesAndMasksInto(msgs, bufs)
	if !reflect.DeepEqual(gotClusters, wantClusters) {
		t.Fatalf("clusters mismatch\n got: %#v\nwant: %#v", gotClusters, wantClusters)
	}
	if !reflect.DeepEqual(gotTemplates, wantTemplates) {
		t.Fatalf("templates mismatch\n got: %#v\nwant: %#v", gotTemplates, wantTemplates)
	}
	if !reflect.DeepEqual(gotMasks, wantMasks) {
		t.Fatalf("masks mismatch\n got: %#v\nwant: %#v", gotMasks, wantMasks)
	}

	short := msgs[:2]
	wantClusters, wantTemplates, wantMasks = parser.ParseWithTemplatesAndMasks(short)
	gotClusters, gotTemplates, gotMasks = parser.ParseWithTemplatesAndMasksInto(short, bufs)
	if !reflect.DeepEqual(gotClusters, wantClusters) {
		t.Fatalf("short clusters mismatch\n got: %#v\nwant: %#v", gotClusters, wantClusters)
	}
	if !reflect.DeepEqual(gotTemplates, wantTemplates) {
		t.Fatalf("short templates mismatch\n got: %#v\nwant: %#v", gotTemplates, wantTemplates)
	}
	if !reflect.DeepEqual(gotMasks, wantMasks) {
		t.Fatalf("short masks mismatch\n got: %#v\nwant: %#v", gotMasks, wantMasks)
	}
}

func TestPatternSampleValidation(t *testing.T) {
	parser := NewParser()
	for _, value := range []float64{0, -0.1, 1.1, math.NaN(), math.Inf(1)} {
		if err := parser.SetPatternSample(value); err == nil {
			t.Fatalf("expected invalid pattern sample error for %v", value)
		}
	}
	for _, value := range []float64{1, 0.5, 0.01} {
		if err := parser.SetPatternSample(value); err != nil {
			t.Fatalf("unexpected error for valid pattern sample %v: %v", value, err)
		}
	}
}

func TestPatternSampleOneParity(t *testing.T) {
	msgs := []string{
		"a x1 x2 b",
		"a x2 b",
		"a x3 b",
		"a x4 b",
		"c x1 d",
		"c x2 d",
	}

	base := NewParser()
	wantClusters, wantTemplates, wantMasks := base.ParseWithTemplatesAndMasks(msgs)

	sampled := NewParser()
	if err := sampled.SetPatternSample(1.0); err != nil {
		t.Fatalf("set pattern sample: %v", err)
	}
	gotClusters, gotTemplates, gotMasks := sampled.ParseWithTemplatesAndMasks(msgs)

	if !reflect.DeepEqual(gotClusters, wantClusters) {
		t.Fatalf("clusters mismatch\n got: %#v\nwant: %#v", gotClusters, wantClusters)
	}
	if !reflect.DeepEqual(gotTemplates, wantTemplates) {
		t.Fatalf("templates mismatch\n got: %#v\nwant: %#v", gotTemplates, wantTemplates)
	}
	if !reflect.DeepEqual(gotMasks, wantMasks) {
		t.Fatalf("masks mismatch\n got: %#v\nwant: %#v", gotMasks, wantMasks)
	}
}

func TestPatternSampleAffectsPatternsNotClusters(t *testing.T) {
	msgs := []string{
		"a x1 x2 b",
		"a x2 b",
		"a x3 b",
		"a x4 b",
		"c x1 d",
		"c x2 d",
		"c x3 d",
		"c x4 d",
	}

	base := NewParser()
	wantClusters, _, _ := base.ParseWithTemplatesAndMasks(msgs)

	sampled := NewParser()
	if err := sampled.SetPatternSample(0.25); err != nil {
		t.Fatalf("set pattern sample: %v", err)
	}
	gotClusters, _, _ := sampled.ParseWithTemplatesAndMasks(msgs)

	if !reflect.DeepEqual(gotClusters, wantClusters) {
		t.Fatalf("clusters mismatch\n got: %#v\nwant: %#v", gotClusters, wantClusters)
	}
}

func TestParallelParseMatchesSerial(t *testing.T) {
	msgs := make([]string, 0, 3000)
	for i := 0; i < 3000; i++ {
		switch i % 3 {
		case 0:
			msgs = append(msgs, fmt.Sprintf("Accepted password for user%d from 10.0.%d.%d port %d ssh2", i, i%256, i%199, 40000+i))
		case 1:
			msgs = append(msgs, fmt.Sprintf("Connection closed by 192.168.%d.%d [preauth]", i%256, i%97))
		default:
			msgs = append(msgs, fmt.Sprintf("block blk_%d received exception java.io.IOException", 100000+i%700))
		}
	}

	serial := NewParser().WithParallelism(1)
	parallel := NewParser().WithParallelism(8)
	sc, st, sm := serial.ParseWithTemplatesAndMasks(msgs)
	pc, pt, pm := parallel.ParseWithTemplatesAndMasks(msgs)

	if !reflect.DeepEqual(sc, pc) {
		t.Fatal("clusters differ between serial and parallel parse")
	}
	if !reflect.DeepEqual(st, pt) {
		t.Fatal("templates differ between serial and parallel parse")
	}
	if !reflect.DeepEqual(sm, pm) {
		t.Fatal("masks differ between serial and parallel parse")
	}
}
