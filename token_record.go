package tipping

type tokenPairID uint64

func newTokenPairID(t1, t2 uint32) tokenPairID {
	if t1 < t2 {
		t1, t2 = t2, t1
	}
	return tokenPairID(uint64(t1)<<32 | uint64(t2))
}

// tokenRecord holds occurrence and co-occurrence counts indexed by the dense
// token IDs assigned by internRows. occ[id] == 0 means the token never passed
// the participation filter.
type tokenRecord struct {
	occ []uint32
	co  map[tokenPairID]uint32
}

type tokenRecordScratch struct {
	seenEpoch []uint32
	epoch     uint32
	ids       []uint32
}

// newTokenRecord counts token occurrences and co-occurrences over the unique
// rows. counts[idx] is the weight of row idx (number of identical input lines
// it represents); nil means weight 1. rowIDs is index-aligned with tokenized.
func newTokenRecord(tokenized [][]Token, rowIDs [][]uint32, counts []uint32, numIDs int, filter staticFilter) *tokenRecord {
	record := &tokenRecord{
		occ: make([]uint32, numIDs),
		co:  map[tokenPairID]uint32{},
	}
	if len(tokenized) == 0 {
		return record
	}

	scratch := tokenRecordScratch{
		seenEpoch: make([]uint32, numIDs),
	}
	for idx := range tokenized {
		w := uint32(1)
		if counts != nil {
			w = counts[idx]
		}
		scratch.epoch++
		epoch := scratch.epoch
		ids := scratch.ids[:0]
		toks := tokenized[idx]
		rids := rowIDs[idx]
		for j, tok := range toks {
			if !filter.keep(tok) {
				continue
			}
			id := rids[j]
			if scratch.seenEpoch[id] == epoch {
				continue
			}
			scratch.seenEpoch[id] = epoch
			ids = append(ids, id)
			record.occ[id] += w
		}
		scratch.ids = ids
		for i := 0; i < len(ids); i++ {
			for j := i + 1; j < len(ids); j++ {
				record.co[newTokenPairID(ids[i], ids[j])] += w
			}
		}
	}

	return record
}
