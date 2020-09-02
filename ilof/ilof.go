// Copyright (C) 2020 Michael J. Fromberger. All Rights Reserved.

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
	"github.com/creachadair/twitter/jhttp"
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
	"lawfareblog":     true, // not itself a human
	"youtube":         true, // streaming service
	"crowdcasthq":     true, // streaming service
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

// UnmarshalJSON decodes a label from a JSON number or string.
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

// MarshalJSON encodes a label to a JSON number or string.
func (a Label) MarshalJSON() ([]byte, error) {
	if _, err := strconv.Atoi(string(a)); err == nil {
		return []byte(a), nil
	}
	return json.Marshal(string(a))
}

// MarshalYAML encodes a label as a YAML number or string.
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

// UnmarshalJSON decodes a date from a JSON string formatted "2006-01-02".
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

// UnmarshalYAML decodes a date from a YAML string formatted "2006-01-02".
func (d *Date) UnmarshalYAML(node *yaml.Node) error {
	ts, err := time.Parse(dateFormat, node.Value)
	if err != nil {
		return err
	}
	*d = Date(ts)
	return nil
}

// MarshalJSON encodes a date as a JSON string.
func (d Date) MarshalJSON() ([]byte, error) {
	return []byte(d.String()), nil
}

// MarshalYAML encodes a date as a YAML string.
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
	cli := twitter.NewClient(&twitter.ClientOpts{
		Authorize: jhttp.BearerTokenAuthorizer(token),
	})
	debug := os.Getenv("TWITTER_DEBUG")
	if debug == "all" {
		cli.Log = func(tag, msg string) { log.Printf("DEBUG %s :: %s", tag, msg) }
	} else if debug != "" {
		tags := make(map[string]bool)
		for _, tag := range strings.Split(debug, ",") {
			tags[tag] = true
		}
		cli.Log = func(tag, msg string) {
			if tags[tag] {
				log.Printf("DEBUG %s :: %s", tag, msg)
			}
		}
	}
	return Twitter{cli: cli}
}

// Updates queries Twitter for episode updates since the specified date.
func (t Twitter) Updates(ctx context.Context, since Date) ([]*TwitterUpdate, error) {
	const query = `from:benjaminwittes "Today on @inlieuoffunshow"`

	// If since corresponds to an air time in the future, there are no further
	// episodes to find. This check averts an error from the API.
	then := time.Time(since).Add(22 * time.Hour)
	if then.After(time.Now()) {
		return nil, errors.New("no matching updates")
	}

	rsp, err := tweets.SearchRecent(query, &tweets.SearchOpts{
		StartTime:  then,
		MaxResults: 10,
		Optional: []types.Fields{
			types.TweetFields{CreatedAt: true, Entities: true},
			types.UserFields{Description: true, ProfileURL: true, Entities: true},
			types.Expansions{types.Expand_MentionUsername},
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
				g.URL = pickUserURL(info)
				g.Notes = info.Description
			}
			up.Guests = append(up.Guests, g)
		}

		ups = append(ups, up)
	}
	return ups, nil
}

func pickUserURL(info *types.User) string {
	if info.Entities == nil {
		return info.ProfileURL
	}
	candidates := append(info.Entities.URL.URLs, info.Entities.Description.URLs...)
	for _, next := range candidates {
		if next.URL == info.ProfileURL && next.Expanded != "" {
			return next.Expanded
		}
	}
	return info.ProfileURL
}

// YouTubeVideoID reports whether s is a YouTube video URL, and if so returns
// the value of its video ID (v) parameter.
func YouTubeVideoID(s string) (string, bool) {
	u, err := url.Parse(s)
	if err != nil {
		return "", false
	}
	switch u.Host {
	case "youtube.com", "www.youtube.com":
		q, err := url.ParseQuery(u.RawQuery)
		if err != nil {
			return "", false
		}
		id := q.Get("v")
		return id, id != ""
	case "youtu.be":
		id := strings.TrimPrefix(u.Path, "/")
		return id, id != ""
	default:
		return "", false
	}
}

// A TwitterUpdate reports data extracted from an episode announcement status
// on Twitter.
type TwitterUpdate struct {
	Date      time.Time // the date of the announcement
	YouTube   string    // if available, the YouTube stream link
	Crowdcast string    // if available, the Crowdcast stream link
	Guests    []*Guest  // if available, possible guest twitter handles
}

// YouTubeVideoInfo returns metadata about the specified YouTube video ID.
func YouTubeVideoInfo(ctx context.Context, id, apiKey string) (*VideoInfo, error) {
	u, err := url.Parse("https://www.googleapis.com/youtube/v3/videos")
	if err != nil {
		return nil, err
	}
	q := make(url.Values)
	q.Set("id", id)
	q.Set("key", apiKey)
	q.Set("part", "snippet")
	u.RawQuery = q.Encode()

	req, err := http.NewRequestWithContext(ctx, "GET", u.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}
	req.Header.Add("Accept", "application/json")
	rsp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}

	var buf bytes.Buffer
	io.Copy(&buf, rsp.Body)
	rsp.Body.Close()
	if rsp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("requst failed: %s", rsp.Status)
	}

	var msg struct {
		Items []struct {
			ID      string     `json:"id"`
			Snippet *VideoInfo `json:"snippet"`
		}
	}
	if err := json.Unmarshal(buf.Bytes(), &msg); err != nil {
		return &VideoInfo{Reply: buf.Bytes()}, err
	}
	for _, item := range msg.Items {
		if item.ID == id {
			item.Snippet.ID = id
			return item.Snippet, nil
		}
	}
	return &VideoInfo{Reply: buf.Bytes()}, errors.New("id not found")
}

// VideoInfo carries metadata about a YouTube video.
type VideoInfo struct {
	ID           string    `json:"-"`
	PublishedAt  time.Time `json:"publishedAt"`
	ChannelID    string    `json:"channelId"`
	ChannelTitle string    `json:"channelTitle"`
	Title        string    `json:"title"`
	Description  string    `json:"description"`

	Reply json.RawMessage `json:"-"`
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
