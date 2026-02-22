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
