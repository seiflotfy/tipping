package tipping

import "sort"

func sharedSlices(messages []string, tokenizer *Tokenizer, filter staticFilter) map[string]struct{} {
	if len(messages) == 0 {
		return map[string]struct{}{}
	}

	var shared map[string]struct{}
	for i, msg := range messages {
		set := make(map[string]struct{})
		for _, tok := range tokenizer.Tokenize(msg) {
			switch tok.Kind {
			case TokenSpecialWhite, TokenWhitespace, TokenSymbolic:
				set[tok.Slice] = struct{}{}
			case TokenAlphabetic:
				if filter.alphabetic {
					set[tok.Slice] = struct{}{}
				}
			case TokenNumeric:
				if filter.numeric {
					set[tok.Slice] = struct{}{}
				}
			case TokenImpure:
				if filter.impure {
					set[tok.Slice] = struct{}{}
				}
			}
		}

		if i == 0 {
			shared = set
			continue
		}
		for slice := range shared {
			if _, ok := set[slice]; !ok {
				delete(shared, slice)
			}
		}
		if len(shared) == 0 {
			break
		}
	}

	if shared == nil {
		return map[string]struct{}{}
	}
	return shared
}

func templatesForCluster(messages []string, tokenizer *Tokenizer, common map[string]struct{}) []string {
	set := make(map[string]struct{})
	for _, msg := range messages {
		toks := tokenizer.Tokenize(msg)
		template := make([]byte, 0, len(msg)+8)
		for _, tok := range toks {
			if _, ok := common[tok.Slice]; ok {
				template = append(template, tok.Slice...)
			} else {
				template = append(template, "<*>"...)
			}
		}
		set[string(template)] = struct{}{}
	}

	out := make([]string, 0, len(set))
	for t := range set {
		out = append(out, t)
	}
	sort.Strings(out)
	return out
}

func parameterMasks(messages []string, tokenizer *Tokenizer, common map[string]struct{}) []string {
	masks := make([]string, len(messages))
	for i, msg := range messages {
		toks := tokenizer.Tokenize(msg)
		mask := make([]byte, 0, len(msg))
		shouldParameterize := false
		for idx, tok := range toks {
			switch tok.Kind {
			case TokenSymbolic:
				if _, ok := common[tok.Slice]; ok {
					if isBoundaryToken(toks, idx+1) {
						mask = append(mask, '0')
					} else if shouldParameterize {
						mask = append(mask, '1')
					} else {
						mask = append(mask, '0')
					}
				} else {
					mask = append(mask, '1')
				}
			case TokenWhitespace:
				mask = append(mask, '0')
				shouldParameterize = false
			case TokenSpecialWhite:
				appendRepeat(&mask, '0', len(tok.Slice))
			case TokenSpecialBlack:
				appendRepeat(&mask, '1', len(tok.Slice))
			default:
				if _, ok := common[tok.Slice]; !ok || shouldParameterize {
					appendRepeat(&mask, '1', len(tok.Slice))
					shouldParameterize = true
				} else {
					appendRepeat(&mask, '0', len(tok.Slice))
				}
			}
		}
		masks[i] = string(mask)
	}
	return masks
}

func isBoundaryToken(tokens []Token, idx int) bool {
	if idx >= len(tokens) {
		return true
	}
	kind := tokens[idx].Kind
	return kind == TokenWhitespace || kind == TokenSymbolic
}

func appendRepeat(buf *[]byte, b byte, n int) {
	for i := 0; i < n; i++ {
		*buf = append(*buf, b)
	}
}
