package tipping

type tokenPair struct {
	a string
	b string
}

func newTokenPair(t1, t2 string) tokenPair {
	if t1 > t2 {
		return tokenPair{a: t1, b: t2}
	}
	return tokenPair{a: t2, b: t1}
}

type tokenRecord struct {
	occ map[string]uint32
	co  map[tokenPair]uint32
}

type tokenSliceScratch struct {
	seen map[tokenKey]struct{}
	toks map[string]struct{}
	out  []string
}

func newTokenRecord(tokenized [][]Token, filter staticFilter) *tokenRecord {
	record := &tokenRecord{occ: map[string]uint32{}, co: map[tokenPair]uint32{}}
	if len(tokenized) == 0 {
		return record
	}

	scratch := tokenSliceScratch{
		seen: make(map[tokenKey]struct{}),
		toks: make(map[string]struct{}),
	}
	for idx := range tokenized {
		toks := uniqueFilteredTokenSlicesWithScratch(tokenized[idx], filter, &scratch)
		for _, tok := range toks {
			record.occ[tok]++
		}
		for i := 0; i < len(toks); i++ {
			for j := i + 1; j < len(toks); j++ {
				record.co[newTokenPair(toks[i], toks[j])]++
			}
		}
	}

	return record
}

func uniqueFilteredTokenSlices(tokens []Token, filter staticFilter) []string {
	scratch := tokenSliceScratch{
		seen: make(map[tokenKey]struct{}, len(tokens)),
		toks: make(map[string]struct{}, len(tokens)),
		out:  make([]string, 0, len(tokens)),
	}
	return uniqueFilteredTokenSlicesWithScratch(tokens, filter, &scratch)
}

func uniqueFilteredTokenSlicesWithScratch(tokens []Token, filter staticFilter, scratch *tokenSliceScratch) []string {
	clear(scratch.seen)
	clear(scratch.toks)
	for _, tok := range tokens {
		id := tokenKeyFor(tok)
		if _, ok := scratch.seen[id]; ok {
			continue
		}
		scratch.seen[id] = struct{}{}
		if !filter.keep(tok) {
			continue
		}
		scratch.toks[tok.Slice] = struct{}{}
	}
	out := scratch.out[:0]
	for tok := range scratch.toks {
		out = append(out, tok)
	}
	scratch.out = out
	return out
}

func (tr *tokenRecord) occurrence(tok string) (uint32, bool) {
	count, ok := tr.occ[tok]
	return count, ok
}

func (tr *tokenRecord) dependency(evidence, condition string) (float64, bool) {
	double, ok := tr.co[newTokenPair(evidence, condition)]
	if !ok {
		return 0, false
	}
	single, ok := tr.occ[evidence]
	if !ok || single == 0 {
		return 0, false
	}
	return float64(double) / float64(single), true
}
