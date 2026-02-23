package tipping

import "testing"

func TestCorpusCodecRoundTrip(t *testing.T) {
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

	codec, err := NewCorpusCodec(msgs, nil)
	if err != nil {
		t.Fatalf("build codec: %v", err)
	}
	if len(codec.Templates()) == 0 {
		t.Fatalf("expected non-empty templates")
	}

	for _, msg := range msgs {
		id, args, ok := codec.Encode(msg)
		if !ok {
			t.Fatalf("expected encode hit for %q", msg)
		}
		idOnly, ok := codec.EncodeID(msg)
		if !ok {
			t.Fatalf("expected encode id hit for %q", msg)
		}
		if idOnly != id {
			t.Fatalf("encode id mismatch for %q: got %d want %d", msg, idOnly, id)
		}

		decoded, ok := codec.Decode(id, args)
		if !ok {
			t.Fatalf("decode failed for %q", msg)
		}
		if decoded != msg {
			t.Fatalf("decode mismatch: got %q want %q", decoded, msg)
		}
	}

	if _, _, ok := codec.Encode("not in corpus"); ok {
		t.Fatalf("expected miss for unknown message")
	}
	if _, ok := codec.EncodeID("not in corpus"); ok {
		t.Fatalf("expected id miss for unknown message")
	}
}

func TestCorpusCodecDuplicateMessagesConsistent(t *testing.T) {
	msgs := []string{
		"svc id=100 path=/a",
		"svc id=100 path=/a",
		"svc id=200 path=/b",
		"svc id=200 path=/b",
	}

	codec, err := NewCorpusCodec(msgs, nil)
	if err != nil {
		t.Fatalf("build codec: %v", err)
	}

	id1, args1, ok := codec.Encode(msgs[0])
	if !ok {
		t.Fatalf("expected hit for first duplicate")
	}
	id2, args2, ok := codec.Encode(msgs[1])
	if !ok {
		t.Fatalf("expected hit for second duplicate")
	}
	if id1 != id2 {
		t.Fatalf("duplicate id mismatch: %d vs %d", id1, id2)
	}
	if !equalStringSlices(args1, args2) {
		t.Fatalf("duplicate args mismatch: %#v vs %#v", args1, args2)
	}
}

func TestCorpusCodecDecodeValidation(t *testing.T) {
	msgs := []string{
		"a x1 b",
		"a x2 b",
	}
	codec, err := NewCorpusCodec(msgs, nil)
	if err != nil {
		t.Fatalf("build codec: %v", err)
	}

	if _, ok := codec.Decode(-1, nil); ok {
		t.Fatalf("expected invalid template id failure")
	}
	if _, ok := codec.Decode(999, nil); ok {
		t.Fatalf("expected out-of-range template id failure")
	}

	id, ok := codec.EncodeID("a x1 b")
	if !ok {
		t.Fatalf("expected known message id")
	}
	if _, ok := codec.Decode(id, nil); ok {
		t.Fatalf("expected args mismatch failure")
	}
}

func TestCorpusCodecHandlesUnclusteredMessages(t *testing.T) {
	msgs := []string{
		"fixed line",
		"totally_unique_12345_value",
	}

	codec, err := NewCorpusCodec(msgs, nil)
	if err != nil {
		t.Fatalf("build codec: %v", err)
	}

	id, args, ok := codec.Encode("totally_unique_12345_value")
	if !ok {
		t.Fatalf("expected unclustered/literal message to be encodable")
	}
	if len(args) != 0 {
		t.Fatalf("expected literal template to have no args, got %#v", args)
	}
	decoded, ok := codec.Decode(id, nil)
	if !ok {
		t.Fatalf("decode failed")
	}
	if decoded != "totally_unique_12345_value" {
		t.Fatalf("decode mismatch: got %q", decoded)
	}
}
