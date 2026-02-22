package tipping

import (
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

	expectedMasks := map[string]string{
		"a x1 x2 b": "001101100",
		"a x2 b":    "001100",
		"a x3 b":    "001100",
		"a x4 b":    "001100",
		"c x1 d":    "001100",
		"c x2 d":    "001100",
		"c x3 d":    "001100",
		"c x4 d":    "001100",
	}
	if !reflect.DeepEqual(masks, expectedMasks) {
		t.Fatalf("masks mismatch\n got: %#v\nwant: %#v", masks, expectedMasks)
	}

	onlyClusters := parser.Parse(msgs)
	if !reflect.DeepEqual(onlyClusters, clusters) {
		t.Fatalf("Parse cluster output drifted")
	}
}
