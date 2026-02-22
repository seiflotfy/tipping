package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/seif/tipping"
)

type oracleInput struct {
	Threshold        float64  `json:"threshold"`
	Symbols          string   `json:"symbols"`
	FilterAlphabetic bool     `json:"filter_alphabetic"`
	FilterNumeric    bool     `json:"filter_numeric"`
	FilterImpure     bool     `json:"filter_impure"`
	SpecialWhites    []string `json:"special_whites"`
	SpecialBlacks    []string `json:"special_blacks"`
	Messages         []string `json:"messages"`
}

type oracleOutput struct {
	Clusters  []int      `json:"clusters"`
	Templates [][]string `json:"templates"`
	Masks     []string   `json:"masks"`
}

const maxOracleInputBytes int64 = 256 * 1024 * 1024

func main() {
	if err := run(os.Stdin, os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(in io.Reader, out io.Writer) error {
	var cfg oracleInput
	limited := &io.LimitedReader{R: in, N: maxOracleInputBytes + 1}
	dec := json.NewDecoder(limited)
	if err := dec.Decode(&cfg); err != nil {
		return fmt.Errorf("decode input json: %w", err)
	}
	if limited.N == 0 {
		return fmt.Errorf("decode input json: input exceeds %d bytes", maxOracleInputBytes)
	}
	if err := dec.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return fmt.Errorf("decode input json: trailing data")
	}

	specialWhites, err := tipping.CompilePatterns(cfg.SpecialWhites)
	if err != nil {
		return fmt.Errorf("compile special_whites: %w", err)
	}
	specialBlacks, err := tipping.CompilePatterns(cfg.SpecialBlacks)
	if err != nil {
		return fmt.Errorf("compile special_blacks: %w", err)
	}

	p := tipping.NewParser().
		WithSymbols(cfg.Symbols).
		WithFilterAlphabetic(cfg.FilterAlphabetic).
		WithFilterNumeric(cfg.FilterNumeric).
		WithFilterImpure(cfg.FilterImpure).
		WithSpecialWhites(specialWhites).
		WithSpecialBlacks(specialBlacks)
	if err := p.SetThreshold(cfg.Threshold); err != nil {
		return fmt.Errorf("invalid threshold: %w", err)
	}

	clusters, templates, masks := p.ParseWithTemplatesAndMasks(cfg.Messages)
	payload := oracleOutput{
		Clusters:  clusters,
		Templates: templates,
		Masks:     masks,
	}

	if err := json.NewEncoder(out).Encode(payload); err != nil {
		return fmt.Errorf("encode output json: %w", err)
	}
	return nil
}
