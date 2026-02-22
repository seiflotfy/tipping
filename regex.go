package tipping

import (
	"regexp"
	"strings"
)

// CompileIntoRegex joins patterns as non-capturing alternatives.
func CompileIntoRegex(patterns ...string) (*regexp.Regexp, error) {
	parts := make([]string, 0, len(patterns))
	for _, p := range patterns {
		parts = append(parts, "(?:"+p+")")
	}
	return regexp.Compile(strings.Join(parts, "|"))
}

// CompilePatterns compiles each pattern into a regex.
func CompilePatterns(patterns []string) ([]*regexp.Regexp, error) {
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
