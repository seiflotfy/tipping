package tipping

import (
	"reflect"
	"testing"
)

func TestMatcherMatchBasic(t *testing.T) {
	m := NewMatcher([]string{
		"a <*> b",
		"c <*> d",
	})

	id, args, ok := m.Match("a x1 b")
	if !ok {
		t.Fatalf("expected a match")
	}
	if id != 0 {
		t.Fatalf("template id mismatch: got %d want %d", id, 0)
	}
	if !reflect.DeepEqual(args, []string{"x1"}) {
		t.Fatalf("args mismatch: got %#v want %#v", args, []string{"x1"})
	}

	if _, _, ok := m.Match("z x1 y"); ok {
		t.Fatalf("expected no match")
	}
}

func TestMatcherMostSpecificWins(t *testing.T) {
	m := NewMatcher([]string{
		"a <*>",
		"a <*> b",
	})

	id, args, ok := m.Match("a x b")
	if !ok {
		t.Fatalf("expected a match")
	}
	if id != 1 {
		t.Fatalf("expected the more specific template, got id=%d", id)
	}
	if !reflect.DeepEqual(args, []string{"x"}) {
		t.Fatalf("args mismatch: got %#v want %#v", args, []string{"x"})
	}
}

func TestMatcherEdgePlaceholders(t *testing.T) {
	m := NewMatcher([]string{
		"<*> error <*>",
		"<*> timeout",
		"<*>",
		"fixed line",
	})

	id, args, ok := m.Match("node-1 error failed")
	if !ok {
		t.Fatalf("expected a match")
	}
	if id != 0 {
		t.Fatalf("template id mismatch: got %d want %d", id, 0)
	}
	if !reflect.DeepEqual(args, []string{"node-1", "failed"}) {
		t.Fatalf("args mismatch: got %#v want %#v", args, []string{"node-1", "failed"})
	}

	id, args, ok = m.Match("fixed line")
	if !ok {
		t.Fatalf("expected literal match")
	}
	if id != 3 {
		t.Fatalf("template id mismatch: got %d want %d", id, 3)
	}
	if len(args) != 0 {
		t.Fatalf("expected no args for literal template, got %#v", args)
	}

	id, args, ok = m.Match("node timeout")
	if !ok {
		t.Fatalf("expected suffix match")
	}
	if id != 1 {
		t.Fatalf("template id mismatch: got %d want %d", id, 1)
	}
	if !reflect.DeepEqual(args, []string{"node"}) {
		t.Fatalf("args mismatch: got %#v want %#v", args, []string{"node"})
	}
}

func TestMatcherFromTemplateSetsAndNormalize(t *testing.T) {
	templateSets := [][]string{
		{"a <*><*> b", "c <*> d"},
		{"c <*> d"},
	}
	m := NewMatcherFromTemplateSets(templateSets)

	templates := m.Templates()
	want := []string{"a <*> b", "c <*> d"}
	if !reflect.DeepEqual(templates, want) {
		t.Fatalf("templates mismatch: got %#v want %#v", templates, want)
	}
}

func TestMatcherMatchIDParity(t *testing.T) {
	m := NewMatcher([]string{
		"a <*> b",
		"c <*> d",
		"fixed line",
	})
	msgs := []string{
		"a x b",
		"c y d",
		"fixed line",
		"no match line",
	}

	for _, msg := range msgs {
		wantID, _, wantOK := m.Match(msg)
		gotID, gotOK := m.MatchID(msg)
		if gotOK != wantOK {
			t.Fatalf("ok mismatch for %q: got %v want %v", msg, gotOK, wantOK)
		}
		if gotID != wantID {
			t.Fatalf("id mismatch for %q: got %d want %d", msg, gotID, wantID)
		}
	}
}

func TestMatcherMatchAllIDs(t *testing.T) {
	m := NewMatcher([]string{
		"a <*> b",
		"c <*> d",
		"fixed line",
	})
	msgs := []string{
		"a x b",
		"a x b",
		"fixed line",
		"c y d",
		"no match line",
		"no match line",
	}

	got := m.MatchAllIDs(msgs)
	want := make([]int, len(msgs))
	for i, msg := range msgs {
		id, ok := m.MatchID(msg)
		if !ok {
			want[i] = -1
			continue
		}
		want[i] = id
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("ids mismatch\n got: %#v\nwant: %#v", got, want)
	}
}
