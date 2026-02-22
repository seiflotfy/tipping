# tipping (Go)

A pure Go implementation of Token Interdependency Parsing (TiPPing), focused on returning log templates.

## Features

- No runtime dependency on Rust.
- Library API for clustering, template extraction, and optional mask generation.
- CLI for batch parsing from file or stdin.
- Golden tests that run standalone.

## Install

```bash
go get <your-module-path>/tipping
```

Replace `<your-module-path>` with your repository module path before release.

## Library usage

```go
package main

import (
	"fmt"
	"tipping"
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
