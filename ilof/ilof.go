// Copyright (C) 2020 Michael J. Fromberger. All Rights Reserved.

// Package ilof provides support code for the In Lieu of Fun site.
package ilof

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"math"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/creachadair/atomicfile"
	"github.com/creachadair/jhttp"
	"github.com/creachadair/twitter"
	"github.com/creachadair/twitter/query"
	"github.com/creachadair/twitter/tweets"
	"github.com/creachadair/twitter/types"
	yaml "gopkg.in/yaml.v3"
)

// BaseURL is the base URL of the production site.
const BaseURL = "https://inlieuof.fun"

// ErrNoUpdates is reported by TwitterUpdates when no updates are available.
var ErrNoUpdates = errors.New("no matching updates")

// KnownUsers is the list of Twitter handles that should not be considered
// candidate guest names, when reading tweets about the show.  Names here
// should be normalized to all-lowercase.
var KnownUsers = map[string]bool{
	"benjaminwittes":  true, // Ben (host)
	"brookingsinst":   true, // Lawfare's supporting institute
	"crowdcast":       true, // streaming service
	"crowdcasthq":     true, // streaming service
	"genevievedfr":    true, // Genevieve (host)
	"inlieuoffunshow": true, // the show account
	"klonick":         true, // Kate (host)
	"lawfareblog":     true, // not itself a human
	"nytimes":         true, // once a newspaper
	"scottjshapiro":   true, // Scott (host)
	"youtube":         true, // streaming service
}

// An Episode records details about an episode of the webcast.
type Episode struct {
	Episode      Label    `json:"episode"`
	Date         Date     `json:"airDate" yaml:"date"`
	Season       int      `json:"season,omitempty" yaml:"season,omitempty"`
	Guests       []string `json:"guestNames,omitempty" yaml:"-"`
	Topics       string   `json:"topics,omitempty" yaml:"topics,omitempty"`
	CrowdcastURL string   `json:"crowdcastURL,omitempty" yaml:"crowdcast,omitempty"`
	YouTubeURL   string   `json:"youTubeURL,omitempty" yaml:"youtube,omitempty"`
	AcastURL     string   `json:"acastURL,omitempty" yaml:"acast,omitempty"`
	AudioFileURL string   `json:"audioFileURL,omitempty" yaml:"audio-file,omitempty"`
	Summary      string   `json:"summary,omitempty" yaml:"summary,omitempty"`
	Special      bool     `json:"special,omitempty" yaml:"special,omitempty"`
	Tags         []string `json:"tags,omitempty" yaml:"tags,flow,omitempty"`
	Links        []*Link  `json:"links,omitempty" yaml:"links,omitempty"`
	Detail       string   `json:"detail,omitempty" yaml:"-"`
}

// HasTag reports whether e has the specified tag.
func (e *Episode) HasTag(tag string) bool {
	for _, t := range e.Tags {
		if t == tag {
			return true
		}
	}
	return false
}

// AddTag adds tag to the tags list for e, if it is not already present.
func (e *Episode) AddTag(tag string) {
	if !e.HasTag(tag) {
		e.Tags = append(e.Tags, tag)
	}
}

// A Label holds the string encoding of an episode label, which can be either a
// number or a string.
type Label string

// Number returns the numeric value of x, or -1 if x is not numeric in form.
func (x Label) Number() float64 {
	if v, err := strconv.ParseFloat(string(x), 64); err == nil {
		return v
	}
	return -1
}

func numToString(f float64) string {
	z, frac := math.Modf(f)
	if frac == 0 {
		return strconv.Itoa(int(z))
	}
	return fmt.Sprintf("%.1f", f)
}

// UnmarshalJSON decodes a label from a JSON number or string.
func (x *Label) UnmarshalJSON(data []byte) error {
	var num float64
	if err := json.Unmarshal(data, &num); err == nil {
		*x = Label(numToString(num))
		return nil
	}
	var s string
	if err := json.Unmarshal(data, &s); err != nil {
		return err
	}
	*x = Label(s)
	return nil
}

// MarshalJSON encodes a label to a JSON number or string.
func (x Label) MarshalJSON() ([]byte, error) {
	if v := x.Number(); v >= 0 {
		return json.Marshal(v)
	}
	return json.Marshal(string(x))
}

// MarshalYAML encodes a label as a YAML number or string.
func (x Label) MarshalYAML() (interface{}, error) {
	if v := x.Number(); v >= 0 {
		return v, nil
	}
	return string(x), nil
}

// A Date records the date when an episode aired or will air.
// It is encoded as a string in the format "YYYY-MM-DD".
type Date time.Time

const dateFormat = "2006-01-02"

func (d Date) String() string { return time.Time(d).Format(dateFormat) }

// UnmarshalText decodes a date from a string formatted "2006-01-02".
func (d *Date) UnmarshalText(data []byte) error {
	ts, err := time.Parse(dateFormat, string(data))
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

// MarshalText encodes a date as a string (used for JSON).
func (d Date) MarshalText() ([]byte, error) {
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
	body, err := io.ReadAll(rsp.Body)
	rsp.Body.Close()
	if err != nil {
		return nil, err
	}
	var ep struct {
		Latest *Episode `json:"latest"`
	}
	if err := json.Unmarshal(body, &ep); err != nil {
		return nil, err
	}
	return ep.Latest, nil
}

// FetchEpisode queries the site for the specified episode.
func FetchEpisode(ctx context.Context, num string) (*Episode, error) {
	rsp, err := http.Get(fmt.Sprintf("%s/episode/%s.json", BaseURL, num))
	if err != nil {
		return nil, err
	}
	if rsp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("request failed: %s", rsp.Status)
	}
	body, err := io.ReadAll(rsp.Body)
	rsp.Body.Close()
	if err != nil {
		return nil, err
	}
	var ep struct {
		*Episode `json:"episode"`
	}
	if err := json.Unmarshal(body, &ep); err != nil {
		return nil, err
	}
	return ep.Episode, nil
}

// AllEpisodes queries the site for all episodes.
func AllEpisodes(ctx context.Context) ([]*Episode, error) {
	rsp, err := http.Get(BaseURL + "/episodes.json")
	if err != nil {
		return nil, err
	}
	body, err := io.ReadAll(rsp.Body)
	rsp.Body.Close()
	if err != nil {
		return nil, err
	}
	var eps struct {
		Episodes []*Episode `json:"episodes"`
	}
	if err := json.Unmarshal(body, &eps); err != nil {
		return nil, err
	}
	return eps.Episodes, nil
}

var epFileName = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}-.*\.md$`)

// ForEachEpisode calls f for each episode file in the given directory.
// If f reports an error, the traversal stops and that error is reported to the
// caller of ForEachEpisode.
func ForEachEpisode(dir string, f func(path string, ep *Episode) error) error {
	ls, err := os.ReadDir(dir)
	if err != nil {
		return fmt.Errorf("listing episodes: %v", err)
	}
	for _, elt := range ls {
		if elt.IsDir() || !epFileName.MatchString(elt.Name()) {
			continue // not an episode file
		}
		path := filepath.Join(dir, elt.Name())
		ep, err := LoadEpisode(path)
		if err != nil {
			return fmt.Errorf("loading episode file: %v", err)
		}
		if err := f(path, ep); err != nil {
			return err
		}
	}
	return nil
}

// newTwitter constructs a twitter client wrapper using the given bearer token.
func newTwitter(token string) *twitter.Client {
	cli := twitter.NewClient(&jhttp.Client{
		Authorize: jhttp.BearerTokenAuthorizer(token),
	})
	v, err := strconv.Atoi(os.Getenv("TWITTER_DEBUG"))
	if err == nil && v > 0 {
		cli.Log = func(tag jhttp.LogTag, msg string) { log.Printf("DEBUG :: %s | %s", tag, msg) }
		cli.LogMask = jhttp.LogTag(v)
	}
	return cli
}

func limitBeforeToday(d Date, limit time.Duration) Date {
	t := time.Time(d)
	now := time.Now().In(t.Location())
	if now.Sub(t) > limit {
		today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
		return Date(today.Add(-limit))
	}
	return d
}

// TwitterUpdates queries Twitter for episode updates since the specified date.
// Updates (if any) are returned in order from oldest to newest.
func TwitterUpdates(ctx context.Context, token string, since Date) ([]*TwitterUpdate, error) {
	b := query.New()
	query := b.And(
		b.Or(
			b.And(
				b.From("benjaminwittes"),
				b.Some("today on", "tonight on", "tomorrow on"),
				b.Mention("inlieuoffunshow"),
			),
			b.And(b.From("inlieuoffunshow"), b.Word("crowdcast")),
		),
		b.HasLinks(),
		b.Not(b.IsReply()),
		b.Not(b.IsRetweet()),
	).String()

	// Twitter limits search history to 7 days unless you have research access.
	// Otherwise, the API will report an error if you try to search earlier.
	// This means we could miss posts if we don't check often enough, but as
	// long as we check once in every 7-day window we should be OK.
	since = limitBeforeToday(since, 7*24*time.Hour)

	// If since corresponds to an air time in the future, there are no further
	// episodes to find. This check averts an error from the API.
	then := time.Time(since).Add(22 * time.Hour)
	if then.After(time.Now()) {
		return nil, ErrNoUpdates
	}

	cli := newTwitter(token)
	rsp, err := tweets.SearchRecent(query, &tweets.SearchOpts{
		StartTime:  then,
		MaxResults: 10,
		Optional: []types.Fields{
			types.TweetFields{CreatedAt: true, Entities: true},
			types.UserFields{Description: true, ProfileURL: true, Entities: true},
			types.Expansions{MentionUsername: true},
		},
	}).Invoke(ctx, cli)
	if err != nil {
		return nil, err
	} else if len(rsp.Tweets) == 0 {
		return nil, ErrNoUpdates
	}
	users, _ := rsp.IncludedUsers()

	var ups []*TwitterUpdate
	for _, tw := range rsp.Tweets {
		up := &TwitterUpdate{
			TweetID: tw.ID,
			Date:    time.Time(*tw.CreatedAt),
			AirDate: time.Time(*tw.CreatedAt),
		}

		// Try to figure out whether this is an update for the current date.
		// If the description includes "tomorrow" we'll assume the air date is
		// one past the posting date.
		if ContainsWord(tw.Text, "tomorrow") {
			up.AirDate = up.Date.AddDate(0, 0, 1)
		}

		// Search URLs for stream links, matched by hostname.
		for _, try := range tw.Entities.URLs {
			u := pickURL(try)
			if u == nil {
				continue
			}
			switch u.Host {
			case "crowdcast.io", "www.crowdcast.io":
				up.Crowdcast = u.String()
			default:
				yt := cleanURL(u).String()
				if id, ok := YouTubeVideoID(yt); ok {
					up.YouTube = fmt.Sprintf("https://www.youtube.com/watch?v=%s", id)
				}
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

		if shouldKeepUpdate(up, ups) {
			ups = append(ups, up)
		}
	}
	for i, j := 0, len(ups)-1; i < j; i++ {
		ups[i], ups[j] = ups[j], ups[i]
		j--
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
	TweetID   string    // the ID of the announcement tweet
	Date      time.Time // the date of the announcement
	AirDate   time.Time // the speculated air date
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
	bits, err := loadRequest(ctx, req)
	if err != nil {
		return nil, err
	}

	var msg struct {
		Items []struct {
			ID      string     `json:"id"`
			Snippet *VideoInfo `json:"snippet"`
		}
	}
	if err := json.Unmarshal(bits, &msg); err != nil {
		return &VideoInfo{Reply: bits}, err
	}
	for _, item := range msg.Items {
		if item.ID == id {
			item.Snippet.ID = id
			return item.Snippet, nil
		}
	}
	return &VideoInfo{Reply: bits}, errors.New("id not found")
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

func parseURL(u string) (*url.URL, error) {
	if u == "" {
		return nil, errors.New("no url")
	}
	return url.Parse(u)
}

func pickURL(u *types.URL) *url.URL {
	if out, err := parseURL(u.Unwound); err == nil {
		return out
	} else if out, err := parseURL(u.Expanded); err == nil {
		return out
	} else if out, err := parseURL(u.URL); err == nil {
		return out
	}
	return nil
}

func cleanURL(u *url.URL) *url.URL {
	q, err := url.ParseQuery(u.RawQuery)
	if err != nil {
		return u
	}
	for key := range q {
		if key != "v" {
			q.Del(key)
		}
	}
	u.RawQuery = q.Encode()
	return u
}

func isSameEpisode(u1, u2 *TwitterUpdate) bool {
	return u1.YouTube == u2.YouTube &&
		u1.Crowdcast == u2.Crowdcast &&
		guestListsEqual(u1.Guests, u2.Guests)
}

func shouldKeepUpdate(u *TwitterUpdate, us []*TwitterUpdate) bool {
	// If the update has no meaningful links, discard it.
	if u.Crowdcast == "" && u.YouTube == "" && len(u.Guests) == 0 {
		return false
	}

	// If we already found an identical update, discard this one.
	for _, v := range us {
		if isSameEpisode(v, u) {
			return false
		}
	}
	return true
}
