package tipping

import (
	"runtime"
	"sync"
)

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

type tokenRecordPartial struct {
	occ map[string]uint32
	co  map[tokenPair]uint32
}

func newTokenRecord(messages []string, tokenizer *Tokenizer, filter staticFilter) *tokenRecord {
	workers := runtime.GOMAXPROCS(0)
	if workers < 1 {
		workers = 1
	}
	if workers > len(messages) && len(messages) > 0 {
		workers = len(messages)
	}
	if len(messages) == 0 {
		return &tokenRecord{occ: map[string]uint32{}, co: map[tokenPair]uint32{}}
	}

	jobs := make(chan string)
	parts := make(chan tokenRecordPartial, workers)

	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			localOcc := make(map[string]uint32)
			localCo := make(map[tokenPair]uint32)
			for msg := range jobs {
				toks := uniqueFilteredTokenSlices(msg, tokenizer, filter)
				for _, tok := range toks {
					localOcc[tok]++
				}
				for i := 0; i < len(toks); i++ {
					for j := i + 1; j < len(toks); j++ {
						localCo[newTokenPair(toks[i], toks[j])]++
					}
				}
			}
			parts <- tokenRecordPartial{occ: localOcc, co: localCo}
		}()
	}

	go func() {
		for _, msg := range messages {
			jobs <- msg
		}
		close(jobs)
		wg.Wait()
		close(parts)
	}()

	record := &tokenRecord{occ: map[string]uint32{}, co: map[tokenPair]uint32{}}
	for part := range parts {
		for tok, count := range part.occ {
			record.occ[tok] += count
		}
		for pair, count := range part.co {
			record.co[pair] += count
		}
	}

	return record
}

func uniqueFilteredTokenSlices(msg string, tokenizer *Tokenizer, filter staticFilter) []string {
	seenTokens := make(map[string]struct{})
	toks := make(map[string]struct{})
	for _, tok := range tokenizer.Tokenize(msg) {
		id := tokenIdentity(tok)
		if _, ok := seenTokens[id]; ok {
			continue
		}
		seenTokens[id] = struct{}{}
		if !filter.keep(tok) {
			continue
		}
		toks[tok.Slice] = struct{}{}
	}
	out := make([]string, 0, len(toks))
	for tok := range toks {
		out = append(out, tok)
	}
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
