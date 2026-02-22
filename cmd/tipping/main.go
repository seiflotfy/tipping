package main

import (
	"bufio"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/seif/tipping"
)

type repeatedString []string

func (r *repeatedString) String() string {
	if r == nil {
		return ""
	}
	return fmt.Sprintf("%v", []string(*r))
}

func (r *repeatedString) Set(v string) error {
	*r = append(*r, v)
	return nil
}

type output struct {
	Clusters  []int      `json:"clusters"`
	Templates [][]string `json:"templates,omitempty"`
	Masks     []string   `json:"masks,omitempty"`
}

const (
	maxScanTokenBytes = 8 * 1024 * 1024
	maxInputMessages  = 1_000_000
	maxInputBytes     = 256 * 1024 * 1024
)

func main() {
	if err := run(os.Args[1:], os.Stdin, os.Stdout, os.Stderr); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(args []string, stdin io.Reader, stdout, stderr io.Writer) error {
	var specialWhites repeatedString
	var specialBlacks repeatedString

	fs := flag.NewFlagSet("tipping", flag.ContinueOnError)
	fs.SetOutput(stderr)

	inputPath := fs.String("input", "-", "input file path ('-' for stdin)")
	outputPath := fs.String("output", "-", "output file path ('-' for stdout)")
	threshold := fs.Float64("threshold", 0.5, "dependency threshold in [0,1]")
	symbols := fs.String("symbols", "", "additional split symbols")
	filterAlphabetic := fs.Bool("filter-alphabetic", true, "include alphabetic tokens in dependency scoring")
	filterNumeric := fs.Bool("filter-numeric", false, "include numeric tokens in dependency scoring")
	filterImpure := fs.Bool("filter-impure", false, "include impure tokens in dependency scoring")
	includeTemplates := fs.Bool("templates", false, "include templates in output")
	includeMasks := fs.Bool("masks", false, "include parameter masks in output")
	fs.Var(&specialWhites, "special-white", "regex that is never parameterized (repeatable)")
	fs.Var(&specialBlacks, "special-black", "regex that is always parameterized (repeatable)")

	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}

	messages, err := readMessages(stdin, *inputPath)
	if err != nil {
		return err
	}

	whiteRegexes, err := tipping.CompilePatterns(specialWhites)
	if err != nil {
		return fmt.Errorf("compile special-white regex: %w", err)
	}
	blackRegexes, err := tipping.CompilePatterns(specialBlacks)
	if err != nil {
		return fmt.Errorf("compile special-black regex: %w", err)
	}

	p := tipping.NewParser().
		WithSpecialWhites(whiteRegexes).
		WithSpecialBlacks(blackRegexes).
		WithSymbols(*symbols).
		WithFilterAlphabetic(*filterAlphabetic).
		WithFilterNumeric(*filterNumeric).
		WithFilterImpure(*filterImpure)
	if err := p.SetThreshold(*threshold); err != nil {
		return err
	}

	var out output
	switch {
	case *includeTemplates && *includeMasks:
		clusters, templates, masks := p.ParseWithTemplatesAndMasks(messages)
		out = output{Clusters: clusters, Templates: templates, Masks: masks}
	case *includeTemplates:
		clusters, templates := p.ParseWithTemplates(messages)
		out = output{Clusters: clusters, Templates: templates}
	case *includeMasks:
		clusters, masks := p.ParseWithMasks(messages)
		out = output{Clusters: clusters, Masks: masks}
	default:
		clusters := p.Parse(messages)
		out = output{Clusters: clusters}
	}

	outWriter, closeOut, err := writerForPath(stdout, *outputPath)
	if err != nil {
		return err
	}
	defer closeOut()

	enc := json.NewEncoder(outWriter)
	enc.SetIndent("", "  ")
	if err := enc.Encode(out); err != nil {
		return fmt.Errorf("encode output: %w", err)
	}

	return nil
}

func readMessages(stdin io.Reader, inputPath string) ([]string, error) {
	in := stdin
	var closer io.Closer
	if inputPath != "-" {
		f, err := os.Open(inputPath)
		if err != nil {
			return nil, fmt.Errorf("open input file: %w", err)
		}
		in = f
		closer = f
	}
	if closer != nil {
		defer closer.Close()
	}

	scanner := bufio.NewScanner(in)
	scanner.Buffer(make([]byte, 0, 64*1024), maxScanTokenBytes)
	messages := make([]string, 0, 1024)
	var totalBytes int64
	for scanner.Scan() {
		if len(messages) >= maxInputMessages {
			return nil, fmt.Errorf("read input: message count exceeds %d", maxInputMessages)
		}
		line := scanner.Text()
		totalBytes += int64(len(line))
		if totalBytes > maxInputBytes {
			return nil, fmt.Errorf("read input: total input bytes exceed %d", maxInputBytes)
		}
		messages = append(messages, line)
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("read input: %w", err)
	}
	return messages, nil
}

func writerForPath(stdout io.Writer, outputPath string) (io.Writer, func() error, error) {
	if outputPath == "-" {
		return stdout, func() error { return nil }, nil
	}
	f, err := os.Create(outputPath)
	if err != nil {
		return nil, nil, fmt.Errorf("open output file: %w", err)
	}
	return f, f.Close, nil
}
