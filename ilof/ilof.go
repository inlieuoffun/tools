// Package ilof provides support code for the In Lieu of Fun site.
package ilof

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/creachadair/atomicfile"
	"github.com/creachadair/twitter"
	"github.com/creachadair/twitter/tweets"
	"github.com/creachadair/twitter/types"
	yaml "gopkg.in/yaml.v3"
)

// BaseURL is the base URL of the production site.
const BaseURL = "https://inlieuof.fun"

// KnownUsers is the list of Twitter handles that should not be considered
// candidate guest names, when reading tweets about the show.  Names here
// should be normalized to all-lowercase.
var KnownUsers = map[string]bool{
	"benjaminwittes":  true, // Ben
	"klonick":         true, // Kate
	"inlieuoffunshow": true, // the show account
}

// An Episode records details about an episode of the webcast.
type Episode struct {
	Episode      Label    `json:"episode"`
	Date         Date     `json:"airDate" yaml:"date"`
	Guests       []string `json:"guestNames,omitempty" yaml:"-"`
	Topics       string   `json:"topics,omitempty" yaml:"topics,omitempty"`
	Summary      string   `json:"summary,omitempty" yaml:"summary,omitempty"`
	CrowdcastURL string   `json:"crowdcastURL,omitempty" yaml:"crowdcast,omitempty"`
	YouTubeURL   string   `json:"youTubeURL,omitempty" yaml:"youtube,omitempty"`
	Links        []*Link  `json:"links,omitempty" yaml:"links,omitempty"`
	Detail       string   `json:"detail,omitempty" yaml:"-"`
}

// A Label holds the string encoding of an episode label, which can be either a
// number or a string.
type Label string

// Int returns the value of a as an integer, or -1.
func (a Label) Int() int {
	if v, err := strconv.Atoi(string(a)); err == nil {
		return v
	}
	return -1
}

func (a *Label) UnmarshalJSON(data []byte) error {
	var z int
	if err := json.Unmarshal(data, &z); err == nil {
		*a = Label(strconv.Itoa(z))
		return nil
	}
	var s string
	if err := json.Unmarshal(data, &s); err != nil {
		return err
	}
	*a = Label(s)
	return nil
}

func (a Label) MarshalJSON() ([]byte, error) {
	if _, err := strconv.Atoi(string(a)); err == nil {
		return []byte(a), nil
	}
	return json.Marshal(string(a))
}

func (a Label) MarshalYAML() (interface{}, error) {
	if v := a.Int(); v >= 0 {
		return v, nil
	}
	return string(a), nil
}

// A Date records the date when an episode aired or will air.
// It is encoded as a string in the format "YYYY-MM-DD".
type Date time.Time

const dateFormat = "2006-01-02"

func (d Date) String() string { return time.Time(d).Format(dateFormat) }

func (d *Date) UnmarshalJSON(data []byte) error {
	var s string
	if err := json.Unmarshal(data, &s); err != nil {
		return err
	}
	ts, err := time.Parse(dateFormat, s)
	if err != nil {
		return err
	}
	*d = Date(ts)
	return nil
}

func (d *Date) UnmarshalYAML(node *yaml.Node) error {
	ts, err := time.Parse(dateFormat, node.Value)
	if err != nil {
		return err
	}
	*d = Date(ts)
	return nil
}

func (d Date) MarshalJSON() ([]byte, error) {
	return []byte(d.String()), nil
}

func (d Date) MarshalYAML() (interface{}, error) {
	return d.String(), nil
}

// A Link records the title and URL of a hyperlink.
type Link struct {
	Title string `json:"title,omitempty" yaml:"title,omitempty"`
	URL   string `json:"url" yaml:"url"`
}

// LoadEpisode loads an episode from the markdown file at path.
func LoadEpisode(path string) (*Episode, error) {
	data, err := ioutil.ReadFile(path)
	if err != nil {
		return nil, err
	}

	// Hacky parse for Jekyll front matter. Actually these are YAML doc headers,
	// but the document handling is too fiddly to bother.
	chunks := strings.SplitN(string(data), "---\n", 3)
	if len(chunks) != 3 || chunks[0] != "" {
		return nil, errors.New("invalid episode file format")
	}

	var ep Episode
	if err := yaml.Unmarshal([]byte(chunks[1]), &ep); err != nil {
		return nil, fmt.Errorf("decoding front matter: %v", err)
	}
	ep.Detail = strings.TrimSpace(chunks[2])
	return &ep, nil
}

// WriteEpisode writes the specified episode to path, overwriting an existing
// file if it exists.
func WriteEpisode(path string, ep *Episode) error {
	f, err := atomicfile.New(path, 0644)
	if err != nil {
		return err
	}
	defer f.Cancel()
	fmt.Fprintln(f, "---")
	data, err := yaml.Marshal(ep)
	if err != nil {
		return err
	}
	f.Write(data)
	fmt.Fprintln(f, "---")
	if ep.Detail != "" {
		fmt.Fprintln(f, ep.Detail)
	}
	return f.Close()
}

// LatestEpisode queries the site for the latest episode.
func LatestEpisode(ctx context.Context) (*Episode, error) {
	rsp, err := http.Get(BaseURL + "/latest.json")
	if err != nil {
		return nil, err
	}
	var buf bytes.Buffer
	io.Copy(&buf, rsp.Body)
	rsp.Body.Close()
	var ep struct {
		Latest *Episode `json:"latest"`
	}
	if err := json.Unmarshal(buf.Bytes(), &ep); err != nil {
		return nil, err
	}
	return ep.Latest, nil
}

// Twitter is a twitter client wrapper for ILoF.
type Twitter struct {
	cli *twitter.Client
}

// NewTwitter constructs a twitter client wrapper using the given bearer token.
func NewTwitter(token string) Twitter {
	cli := &twitter.Client{Authorize: twitter.BearerTokenAuthorizer(token)}
	b, _ := strconv.ParseBool(os.Getenv("TWITTER_DEBUG"))
	if b {
		cli.Log = func(tag, msg string) {
			log.Printf("TWITTER DEBUG %s :: %s", tag, msg)
		}
	}
	return Twitter{cli: cli}
}

// Updates queries Twitter for episode updates since the specified date.
func (t Twitter) Updates(ctx context.Context, since Date) ([]*TwitterUpdate, error) {
	const query = `from:benjaminwittes "Today on @inlieuoffunshow"`
	rsp, err := tweets.SearchRecent(query, &tweets.SearchOpts{
		StartTime:  time.Time(since).Add(22 * time.Hour),
		MaxResults: 10,
		TweetFields: []string{
			types.Tweet_CreatedAt,
			types.Tweet_Entities, // for URLs, usernames
		},
		Expansions: []string{
			types.ExpandMentionUsername,
		},
	}).Invoke(ctx, t.cli)
	if err != nil {
		return nil, err
	} else if len(rsp.Tweets) == 0 {
		return nil, errors.New("no matching updates")
	}
	users, _ := rsp.IncludedUsers()

	var ups []*TwitterUpdate
	for _, tw := range rsp.Tweets {
		up := &TwitterUpdate{Date: time.Time(*tw.CreatedAt)}

		// Search URLs for stream links, matched by hostname.
		for _, try := range tw.Entities.URLs {
			u := pickURL(try)
			if u == nil {
				continue
			}
			switch u.Host {
			case "crowdcast.io", "www.crowdcast.io":
				up.Crowdcast = u.String()
			case "youtube.com", "www.youtube.com", "youtu.be":
				cleanURL(u)
				up.YouTube = u.String()
			}
		}

		// Find mentions not recorded in the stop list.
		for _, m := range tw.Entities.Mentions {
			if KnownUsers[strings.ToLower(m.Username)] {
				continue // this is not a guest
			}
			g := &Guest{Twitter: m.Username}
			if info := users.FindByUsername(m.Username); info != nil {
				g.Name = info.Name
				g.URL = info.ProfileURL
			}
			up.Guests = append(up.Guests, g)
		}

		ups = append(ups, up)
	}
	return ups, nil
}

// A TwitterUpdate reports data extracted from an episode announcement status
// on Twitter.
type TwitterUpdate struct {
	Date      time.Time // the date of the announcement
	YouTube   string    // if available, the YouTube stream link
	Crowdcast string    // if available, the Crowdcast stream link
	Guests    []*Guest  // if available, possible guest twitter handles
}

func pickURL(u *types.URL) *url.URL {
	if out, err := url.Parse(u.Unwound); err == nil {
		return out
	} else if out, err := url.Parse(u.Expanded); err == nil {
		return out
	} else if out, err := url.Parse(u.URL); err == nil {
		return out
	}
	return nil
}

func cleanURL(u *url.URL) {
	q, err := url.ParseQuery(u.RawQuery)
	if err != nil {
		return
	}
	for key := range q {
		if key != "v" {
			q.Del(key)
		}
	}
	u.RawQuery = q.Encode()
}
