package ilof

import (
	"math"
	"strings"

	"bitbucket.org/creachadair/stringset"
)

// Similarity computes a Otsuka-Ochiai coefficient for the words in a and b.
func Similarity(a, b string) float64 {
	wa := stringset.New(Words(a)...)
	wb := stringset.New(Words(b)...)
	if wa.Empty() && wb.Empty() {
		return 1
	}
	num := float64(wa.Intersect(wb).Len())
	den := float64(wa.Len() * wb.Len())
	if den == 0 {
		return 0
	}
	return num / math.Sqrt(den)
}

// ContainsWord reports whether s contains word.
func ContainsWord(s, word string) bool {
	return stringset.Contains(Words(s), strings.ToLower(word))
}

// Words parses s into a bag of words. Words are separated by whitespace and
// normalized to lower-case.
func Words(s string) []string {
	return strings.Fields(strings.TrimSpace(strings.ToLower(s)))
}
