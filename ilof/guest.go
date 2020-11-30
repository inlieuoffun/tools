// Copyright (C) 2020 Michael J. Fromberger. All Rights Reserved.

package ilof

import (
	"fmt"
	"io/ioutil"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/creachadair/atomicfile"
	yaml "gopkg.in/yaml.v3"
)

var (
	easternTime *time.Location

	// The start time of the first (pilot) episode.
	firstShow = time.Date(2020, 3, 25, 21, 0, 0, 0, time.UTC)
)

func init() {
	usEast, err := time.LoadLocation("America/New_York")
	if err != nil {
		panic(err)
	}
	easternTime = usEast
}

// A Guest gives the name and some links for a guest.
type Guest struct {
	Name     string `yaml:"name"`
	Twitter  string `yaml:"twitter,omitempty"`
	URL      string `yaml:"url,omitempty"`
	Notes    string `yaml:"notes,omitempty"`
	Episodes []int  `yaml:"episodes,flow"`
}

func (g *Guest) String() string {
	var buf strings.Builder
	buf.WriteString(g.Name)
	if g.URL != "" {
		fmt.Fprintf(&buf, " <%s>", g.URL)
	}
	if g.Twitter != "" {
		fmt.Fprintf(&buf, " (@%s)", g.Twitter)
	}
	if len(g.Episodes) != 0 {
		fmt.Fprintf(&buf, " %+v", g.Episodes)
	}
	return buf.String()
}

// OnEpisode reports whether g is a guest on the specified episode.
func (g *Guest) OnEpisode(ep int) bool {
	for _, v := range g.Episodes {
		if v == ep {
			return true
		}
	}
	return false
}

var firstNonComment = regexp.MustCompile(`(?m)^[^#]`)

// AddOrUpdateGuests updates the guest list at path for the listed guests on
// the specified episode. New entries are added if they do not already exist,
// matched by name. Otherwise, new episode entries are added to existing
// guests. If successful, the file at path is updated in place.
func AddOrUpdateGuests(episode int, path string, guests []*Guest) error {
	if len(guests) == 0 {
		return nil
	}

	data, err := ioutil.ReadFile(path)
	if err != nil {
		return err
	}

	// Cut off and save the comment block at the top of the file, so we can put
	// it back when the file is updated.
	var comments, content []byte = nil, data
	if m := firstNonComment.FindIndex(data); m != nil {
		comments = data[:m[0]]
		content = data[m[0]:]
	}

	var entries []*Guest
	if err := yaml.Unmarshal(content, &entries); err != nil {
		return err
	}

	dirty := false
	for _, g := range guests {
		old := findGuest(g, entries)
		if old == nil {
			g.Episodes = []int{episode}
			entries = append(entries, g)
			dirty = true
		} else if !old.OnEpisode(episode) {
			old.Episodes = append(old.Episodes, episode)
			sort.Ints(old.Episodes)
			dirty = true
		}
	}

	if !dirty {
		return nil // no changes; don't rewrite the file
	}

	out, err := atomicfile.New(path, 0644)
	if err != nil {
		return err
	}
	defer out.Cancel()
	out.Write(comments)

	// Write out each record separately, so we can keep space between them for
	// the benefit of human readers. There must be a better way to do this.
	for i := range entries {
		if i > 0 {
			fmt.Fprintln(out)
		}
		bits, err := yaml.Marshal(entries[i : i+1])
		if err != nil {
			return err
		}
		out.Write(bits)
	}
	return out.Close()
}

func findGuest(needle *Guest, gs []*Guest) *Guest {
	for _, g := range gs {
		if g.Name == needle.Name || g.Twitter != "" && g.Twitter == needle.Twitter {
			return g
		}
	}
	return nil
}

// MysteryGuestSunday reports whether today is Sunday, and if so whose turn it
// is to host the guest.
func MysteryGuestSunday() (ok bool, who string) {
	now := time.Now().In(easternTime)
	diff := now.Sub(firstShow) / (7 * 24 * time.Hour) // nominal weeks
	if diff%2 == 0 {
		who = "Ben"
	} else {
		who = "Kate"
	}
	return now.Weekday() == time.Sunday, who
}
