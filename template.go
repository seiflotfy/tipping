package tipping

import "sort"

func sharedSlices(tokenized [][]Token, filter staticFilter) map[string]struct{} {
	if len(tokenized) == 0 {
		return map[string]struct{}{}
	}

	shared := make(map[string]struct{}, len(tokenized[0]))
	for _, tok := range tokenized[0] {
		if commonCandidate(tok, filter) {
			shared[tok.Slice] = struct{}{}
		}
	}
	if len(tokenized) == 1 || len(shared) == 0 {
		return shared
	}

	seenEpoch := make(map[string]uint32, len(shared))
	epoch := uint32(1)
	for i := 1; i < len(tokenized); i++ {
		toks := tokenized[i]

		for _, tok := range toks {
			if !commonCandidate(tok, filter) {
				continue
			}
			if _, ok := shared[tok.Slice]; ok {
				seenEpoch[tok.Slice] = epoch
			}
		}

		for slice := range shared {
			if seenEpoch[slice] != epoch {
				delete(shared, slice)
			}
		}
		if len(shared) == 0 {
			break
		}
		epoch++
		if epoch == 0 {
			clear(seenEpoch)
			epoch = 1
		}
	}

	return shared
}

func commonCandidate(tok Token, filter staticFilter) bool {
	switch tok.Kind {
	case TokenSpecialWhite, TokenWhitespace, TokenSymbolic:
		return true
	case TokenAlphabetic:
		return filter.alphabetic
	case TokenNumeric:
		return filter.numeric
	case TokenImpure:
		return filter.impure
	default:
		return false
	}
}

func templatesForCluster(tokenized [][]Token, common map[string]struct{}) []string {
	set := make(map[string]struct{}, len(tokenized))
	for _, toks := range tokenized {
		template := make([]byte, 0, tokenLength(toks)+8)
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

func parameterMasks(tokenized [][]Token, common map[string]struct{}) []string {
	masks := make([]string, len(tokenized))
	for i, toks := range tokenized {
		mask := make([]byte, 0, tokenLength(toks))
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

func tokenLength(tokens []Token) int {
	n := 0
	for _, tok := range tokens {
		n += len(tok.Slice)
	}
	return n
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
