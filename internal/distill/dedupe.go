package distill

import (
	"strings"
	"unicode"

	"golang.org/x/text/runes"
	"golang.org/x/text/transform"
	"golang.org/x/text/unicode/norm"
)

const dupThreshold = 0.6

// normalize: minúsculas, sem acentos, sem pontuação, espaços colapsados.
func normalize(s string) string {
	t := transform.Chain(norm.NFD, runes.Remove(runes.In(unicode.Mn)), norm.NFC)
	out, _, err := transform.String(t, s)
	if err != nil {
		out = s
	}
	out = strings.ToLower(out)
	var b strings.Builder
	prevSpace := false
	for _, r := range out {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
			prevSpace = false
		} else if !prevSpace {
			b.WriteRune(' ')
			prevSpace = true
		}
	}
	return strings.TrimSpace(b.String())
}

// stopWords são palavras funcionais removidas antes do cálculo de similaridade.
var stopWords = map[string]bool{
	"de": true, "do": true, "da": true, "no": true, "na": true, "em": true,
	"o": true, "a": true, "os": true, "as": true, "e": true, "para": true,
	"com": true, "um": true, "uma": true, "the": true, "of": true, "in": true,
	"on": true, "at": true, "to": true, "and": true, "or": true, "for": true,
}

func tokenSet(s string) map[string]struct{} {
	m := map[string]struct{}{}
	for _, w := range strings.Fields(normalize(s)) {
		if !stopWords[w] {
			m[w] = struct{}{}
		}
	}
	return m
}

// similarity: Jaccard sobre tokens (interseção/união).
func similarity(a, b string) float64 {
	sa, sb := tokenSet(a), tokenSet(b)
	if len(sa) == 0 || len(sb) == 0 {
		return 0
	}
	inter := 0
	for w := range sa {
		if _, ok := sb[w]; ok {
			inter++
		}
	}
	union := len(sa) + len(sb) - inter
	if union == 0 {
		return 0
	}
	return float64(inter) / float64(union)
}

func IsDuplicate(a, b string) bool { return similarity(a, b) >= dupThreshold }
