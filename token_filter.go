package tipping

type staticFilter struct {
	alphabetic bool
	numeric    bool
	impure     bool
}

func newStaticFilter(alphabetic, numeric, impure bool) staticFilter {
	return staticFilter{
		alphabetic: alphabetic,
		numeric:    numeric,
		impure:     impure,
	}
}

func (f staticFilter) keep(tok Token) bool {
	switch tok.Kind {
	case TokenAlphabetic:
		return f.alphabetic
	case TokenNumeric:
		return f.numeric
	case TokenImpure:
		return f.impure
	case TokenSpecialWhite:
		return true
	default:
		return false
	}
}
