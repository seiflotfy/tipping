package tipping

import "sort"

// commonSet answers "is this token id common to every sampled row of the
// cluster" with one array read. mark is shared across clusters and cleared by
// epoch advancement, never by rewriting.
type commonSet struct {
	mark  []uint32
	final uint32
}

func (c *commonSet) contains(id uint32) bool {
	return c.mark[id] == c.final
}

type commonScratch struct {
	mark []uint32
	base uint32
}

// sharedSlices intersects the candidate tokens across all rows. tokenIDs is
// index-aligned with tokenized. The returned set is only valid until the next
// call with the same scratch.
func sharedSlices(tokenized [][]Token, tokenIDs [][]uint32, filter staticFilter, scratch *commonScratch, numIDs int) commonSet {
	if cap(scratch.mark) < numIDs {
		scratch.mark = make([]uint32, numIDs)
		scratch.base = 0
	} else {
		scratch.mark = scratch.mark[:numIDs]
	}
	out := commonSet{mark: scratch.mark}
	if len(tokenized) == 0 {
		return out
	}
	if scratch.base > ^uint32(0)-uint32(len(tokenized))-1 {
		clear(scratch.mark)
		scratch.base = 0
	}
	base := scratch.base

	count := 0
	for j, tok := range tokenized[0] {
		if !commonCandidate(tok, filter) {
			continue
		}
		id := tokenIDs[0][j]
		if scratch.mark[id] != base+1 {
			scratch.mark[id] = base + 1
			count++
		}
	}

	final := base + 1
	for i := 1; i < len(tokenized); i++ {
		if count == 0 {
			break
		}
		epoch := base + uint32(i)
		hits := 0
		for j, tok := range tokenized[i] {
			if !commonCandidate(tok, filter) {
				continue
			}
			id := tokenIDs[i][j]
			if scratch.mark[id] == epoch {
				scratch.mark[id] = epoch + 1
				hits++
			}
		}
		count = hits
		final = epoch + 1
	}

	scratch.base = base + uint32(len(tokenized)) + 1
	if count == 0 {
		// Nothing survived: point final at an epoch no mark can hold.
		final = scratch.base
	}
	out.final = final
	return out
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

func templatesForCluster(tokenized [][]Token, tokenIDs [][]uint32, common commonSet) []string {
	set := make(map[string]struct{}, len(tokenized))
	var buf []byte
	for i, toks := range tokenized {
		buf = buf[:0]
		lastPlaceholder := false
		for j, tok := range toks {
			if common.contains(tokenIDs[i][j]) {
				buf = append(buf, tok.Slice...)
				lastPlaceholder = false
			} else {
				if lastPlaceholder {
					continue
				}
				buf = append(buf, "<*>"...)
				lastPlaceholder = true
			}
		}
		if _, ok := set[string(buf)]; !ok {
			set[string(buf)] = struct{}{}
		}
	}

	out := make([]string, 0, len(set))
	for t := range set {
		out = append(out, t)
	}
	sort.Strings(out)
	return out
}

func parameterMasks(tokenized [][]Token, tokenIDs [][]uint32, common commonSet) []string {
	masks := make([]string, len(tokenized))
	var buf []byte
	for i, toks := range tokenized {
		total := 0
		for _, tok := range toks {
			total += len(tok.Slice)
		}
		if cap(buf) < total {
			buf = make([]byte, total)
		} else {
			buf = buf[:total]
		}
		pos := 0
		for j, tok := range toks {
			c := byte('1')
			if common.contains(tokenIDs[i][j]) {
				c = '0'
			}
			end := pos + len(tok.Slice)
			fill := buf[pos:end]
			for k := range fill {
				fill[k] = c
			}
			pos = end
		}
		masks[i] = string(buf)
	}
	return masks
}
