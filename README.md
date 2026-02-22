# tipping (Go)

A pure Go implementation of Token Interdependency Parsing (TiPPing), focused on returning log templates.

## Features

- No runtime dependency on Rust.
- Library API for clustering, template extraction, and optional mask generation.
- CLI for batch parsing from file or stdin.
- Golden tests that run standalone.

## Install

```bash
go get github.com/seif/tipping
```

## Library usage

```go
package main

import (
	"fmt"
	"github.com/seif/tipping"
)

func main() {
	msgs := []string{
		"a x1 x2 b",
		"a x2 b",
		"c x1 d",
	}

	clusters, templates := tipping.NewParser().
		ParseWithTemplates(msgs)

	fmt.Println(clusters)
	fmt.Println(templates)
}
```

## CLI usage

```bash
# Read from stdin and emit clusters + templates
cat logs.txt | go run ./cmd/tipping -templates

# Read from file and write JSON to file
go run ./cmd/tipping -input logs.txt -output result.json -templates
```

If you also want masks, add `-masks`.

## Matching semantics

`ParseWithTemplates` returns:

- `clusters []int`: cluster id for each input message index.
- `templates [][]string`: template set per cluster id.

For message `msgs[i]`, its matched template set is `templates[clusters[i]]`
when `clusters[i] >= 0`.

`clusters[i] == -1` means the message was left unclustered.

`ParseWithMasks` returns:

- `clusters []int`: cluster id for each input message index.
- `masks []string`: parameter mask aligned by input index (`masks[i]` belongs to `msgs[i]`).

Masks are index-aligned, so duplicate input lines remain duplicated in output.

See all options:

```bash
go run ./cmd/tipping -h
```

## Testing

```bash
go test ./...
```

This runs fully without Rust.

Optional external Go oracle check:

```bash
TIPPING_VERIFY_GO_ORACLE=1 go test ./... -run TestGoldenParity
```

Update goldens:

```bash
# Update from Go output only
TIPPING_UPDATE_GOLDEN=1 go test ./... -run TestGoldenParity

# Update and verify against the Go oracle
TIPPING_UPDATE_GOLDEN=1 TIPPING_VERIFY_GO_ORACLE=1 go test ./... -run TestGoldenParity
```

## Benchmarks and profiling

Run parser/tokenizer benchmarks:

```bash
go test -run '^$' -bench Benchmark -benchmem ./...
```

Capture CPU + memory profiles for the heaviest parser benchmark:

```bash
go test -run '^$' -bench '^BenchmarkParseWithTemplatesAndMasksSpecial4K$' -benchmem -benchtime=3s -cpu 1 -cpuprofile cpu.out -memprofile mem.out .
go tool pprof -top cpu.out
go tool pprof -top -alloc_space mem.out
go tool pprof -top -alloc_objects mem.out
```
