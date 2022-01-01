package ilof

import (
	"math"
	"strings"

	"bitbucket.org/creachadair/stringset"
)

// Similarity computes a Otsuka-Ochiai coefficient for the words in a and b.
func Similarity(a, b string) float64 {
	wa := stringset.New(strings.Fields(strings.ToLower(a))...)
	wb := stringset.New(strings.Fields(strings.ToLower(b))...)
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
