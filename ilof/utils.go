package ilof

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"math"
	"net/http"
	"regexp"
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

var punct = regexp.MustCompile(`\W+`)

// Words parses s into a bag of words. Words are separated by whitespace and
// normalized to lower-case.
func Words(s string) []string {
	var words []string
	for _, w := range strings.Fields(strings.TrimSpace(strings.ToLower(s))) {
		words = append(words, punct.ReplaceAllString(w, ""))
	}
	return words
}

func loadRequest(ctx context.Context, req *http.Request) ([]byte, error) {
	rsp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	var buf bytes.Buffer
	io.Copy(&buf, rsp.Body)
	rsp.Body.Close()
	if rsp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("request failed: %s", rsp.Status)
	}
	return buf.Bytes(), nil
}
