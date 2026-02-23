package tipping

type tokenPairID uint64

func newTokenPairID(t1, t2 uint32) tokenPairID {
	if t1 < t2 {
		t1, t2 = t2, t1
	}
	return tokenPairID(uint64(t1)<<32 | uint64(t2))
}

type tokenRecord struct {
	tokenID map[string]uint32
	occ     []uint32
	co      map[tokenPairID]uint32
}

type tokenSliceScratch struct {
	seen map[tokenKey]struct{}
	toks map[string]struct{}
	out  []string
	ids  []uint32
}

func newTokenRecord(tokenized [][]Token, filter staticFilter) *tokenRecord {
	record := &tokenRecord{
		tokenID: map[string]uint32{},
		occ:     make([]uint32, 1),
		co:      map[tokenPairID]uint32{},
	}
	if len(tokenized) == 0 {
		return record
	}

	scratch := tokenSliceScratch{
		seen: make(map[tokenKey]struct{}),
		toks: make(map[string]struct{}),
	}
	for idx := range tokenized {
		toks := uniqueFilteredTokenSlicesWithScratch(tokenized[idx], filter, &scratch)
		ids := scratch.ids[:0]
		for _, tok := range toks {
			id := record.internToken(tok)
			ids = append(ids, id)
			record.occ[id]++
		}
		scratch.ids = ids
		for i := 0; i < len(ids); i++ {
			for j := i + 1; j < len(ids); j++ {
				record.co[newTokenPairID(ids[i], ids[j])]++
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
	id, ok := tr.tokenID[tok]
	if !ok {
		return 0, false
	}
	if int(id) >= len(tr.occ) {
		return 0, false
	}
	return tr.occ[id], true
}

func (tr *tokenRecord) dependency(evidence, condition string) (float64, bool) {
	evidenceID, ok := tr.tokenID[evidence]
	if !ok {
		return 0, false
	}
	conditionID, ok := tr.tokenID[condition]
	if !ok {
		return 0, false
	}
	double, ok := tr.co[newTokenPairID(evidenceID, conditionID)]
	if !ok {
		return 0, false
	}
	if int(evidenceID) >= len(tr.occ) {
		return 0, false
	}
	single := tr.occ[evidenceID]
	if single == 0 {
		return 0, false
	}
	return float64(double) / float64(single), true
}

func (tr *tokenRecord) tokenIDOf(tok string) (uint32, bool) {
	id, ok := tr.tokenID[tok]
	return id, ok
}

func (tr *tokenRecord) internToken(tok string) uint32 {
	if id, ok := tr.tokenID[tok]; ok {
		return id
	}
	id := uint32(len(tr.tokenID) + 1)
	tr.tokenID[tok] = id
	tr.occ = append(tr.occ, 0)
	return id
}
