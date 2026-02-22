package tipping

import (
	"reflect"
	"regexp"
	"testing"
)

func TestTokenizerPreTokenize(t *testing.T) {
	tokenizer := NewTokenizer(
		[]*regexp.Regexp{regexp.MustCompile(`\ba\b`)},
		[]*regexp.Regexp{regexp.MustCompile(`\d+\.\d+`)},
		map[rune]struct{}{},
	)

	computed := tokenizer.preTokenize("This 10001.2 is 1.323 a 1.4411 message")
	expected := []preToken{
		{kind: preTokenUnrefined, slice: "This "},
		{kind: preTokenSpecialBlack, slice: "10001.2"},
		{kind: preTokenUnrefined, slice: " is "},
		{kind: preTokenSpecialBlack, slice: "1.323"},
		{kind: preTokenUnrefined, slice: " "},
		{kind: preTokenSpecialWhite, slice: "a"},
		{kind: preTokenUnrefined, slice: " "},
		{kind: preTokenSpecialBlack, slice: "1.4411"},
		{kind: preTokenUnrefined, slice: " message"},
	}

	if !reflect.DeepEqual(computed, expected) {
		t.Fatalf("pre-tokenize mismatch\n got: %#v\nwant: %#v", computed, expected)
	}
}

func TestTokenizerTokenize(t *testing.T) {
	tokenizer := NewTokenizer(
		[]*regexp.Regexp{regexp.MustCompile(`fan_\d+`)},
		[]*regexp.Regexp{regexp.MustCompile(`\d+\.\d+`)},
		newSymbolSet("."),
	)

	computed := tokenizer.Tokenize("Fan fan_2 speed is set to 12.3114 on machine sys.node.fan_3 on node 12")
	expected := []Token{
		{Kind: TokenAlphabetic, Slice: "Fan"},
		{Kind: TokenWhitespace, Slice: " "},
		{Kind: TokenSpecialWhite, Slice: "fan_2"},
		{Kind: TokenWhitespace, Slice: " "},
		{Kind: TokenAlphabetic, Slice: "speed"},
		{Kind: TokenWhitespace, Slice: " "},
		{Kind: TokenAlphabetic, Slice: "is"},
		{Kind: TokenWhitespace, Slice: " "},
		{Kind: TokenAlphabetic, Slice: "set"},
		{Kind: TokenWhitespace, Slice: " "},
		{Kind: TokenAlphabetic, Slice: "to"},
		{Kind: TokenWhitespace, Slice: " "},
		{Kind: TokenSpecialBlack, Slice: "12.3114"},
		{Kind: TokenWhitespace, Slice: " "},
		{Kind: TokenAlphabetic, Slice: "on"},
		{Kind: TokenWhitespace, Slice: " "},
		{Kind: TokenAlphabetic, Slice: "machine"},
		{Kind: TokenWhitespace, Slice: " "},
		{Kind: TokenAlphabetic, Slice: "sys"},
		{Kind: TokenSymbolic, Slice: "."},
		{Kind: TokenAlphabetic, Slice: "node"},
		{Kind: TokenSymbolic, Slice: "."},
		{Kind: TokenSpecialWhite, Slice: "fan_3"},
		{Kind: TokenWhitespace, Slice: " "},
		{Kind: TokenAlphabetic, Slice: "on"},
		{Kind: TokenWhitespace, Slice: " "},
		{Kind: TokenAlphabetic, Slice: "node"},
		{Kind: TokenWhitespace, Slice: " "},
		{Kind: TokenNumeric, Slice: "12"},
	}

	if !reflect.DeepEqual(computed, expected) {
		t.Fatalf("tokenize mismatch\n got: %#v\nwant: %#v", computed, expected)
	}
}

func TestCompileIntoRegex(t *testing.T) {
	re, err := CompileIntoRegex(`\d+`, `[a-zA-Z]+`)
	if err != nil {
		t.Fatalf("compile failed: %v", err)
	}
	if !re.MatchString("123") {
		t.Fatalf("expected number to match")
	}
	if !re.MatchString("abc") {
		t.Fatalf("expected lowercase to match")
	}
	if !re.MatchString("ABC") {
		t.Fatalf("expected uppercase to match")
	}
	if re.MatchString("@") {
		t.Fatalf("did not expect @ to match")
	}
	if re.MatchString("#") {
		t.Fatalf("did not expect # to match")
	}
}
