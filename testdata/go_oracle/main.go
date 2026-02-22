package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"regexp"

	"tipping"
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
	Clusters  []int             `json:"clusters"`
	Templates [][]string        `json:"templates"`
	Masks     map[string]string `json:"masks"`
}

func main() {
	if err := run(os.Stdin, os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(in io.Reader, out io.Writer) error {
	var cfg oracleInput
	if err := json.NewDecoder(in).Decode(&cfg); err != nil {
		return fmt.Errorf("decode input json: %w", err)
	}

	specialWhites, err := compilePatterns(cfg.SpecialWhites)
	if err != nil {
		return fmt.Errorf("compile special_whites: %w", err)
	}
	specialBlacks, err := compilePatterns(cfg.SpecialBlacks)
	if err != nil {
		return fmt.Errorf("compile special_blacks: %w", err)
	}

	p := tipping.NewParser().
		WithThreshold(cfg.Threshold).
		WithSymbols(cfg.Symbols).
		WithFilterAlphabetic(cfg.FilterAlphabetic).
		WithFilterNumeric(cfg.FilterNumeric).
		WithFilterImpure(cfg.FilterImpure).
		WithSpecialWhites(specialWhites).
		WithSpecialBlacks(specialBlacks)

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

func compilePatterns(patterns []string) ([]*regexp.Regexp, error) {
	out := make([]*regexp.Regexp, 0, len(patterns))
	for _, pattern := range patterns {
		re, err := regexp.Compile(pattern)
		if err != nil {
			return nil, err
		}
		out = append(out, re)
	}
	return out, nil
}
