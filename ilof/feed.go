package ilof

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"

	"github.com/mmcdole/gofeed"
	ext "github.com/mmcdole/gofeed/extensions"
	"golang.org/x/net/html"
	"golang.org/x/net/html/atom"
)

// AcastFeedURL is the URL of the Acast RSS feed for ILoF.
const AcastFeedURL = "https://feeds.acast.com/public/shows/in-lieu-of-fun"

// LoadAcastFeed fetches and parses the Acast RSS feed from url.
func LoadAcastFeed(ctx context.Context, url string) ([]*AudioEpisode, error) {
	p := gofeed.NewParser()
	// Yes, the parser API has the context backward.
	feed, err := p.ParseURLWithContext(url, ctx)
	if err != nil {
		return nil, fmt.Errorf("parsing feed: %w", err)
	}

	// Extract the show URL, since the episode may not correctly link back to
	// its landing page because humans are bad at details.
	showName := getExtensionField(feed.Extensions, "acast", "showUrl")

	var eps []*AudioEpisode
	for _, item := range feed.Items {
		ep, err := newAudioEpisode(showName, item)
		if err != nil {
			return nil, fmt.Errorf("extracting episode: %w", err)
		}
		eps = append(eps, ep)
	}
	return eps, nil
}

func getExtensionField(ext ext.Extensions, ns, name string) string {
	es := ext[ns][name]
	if es == nil {
		return ""
	}
	for _, e := range es {
		if e.Name == name {
			return e.Value
		}
	}
	return ""
}

// AudioEpisode represents metadata about an audio recording of an ILoF episode
// on Acast, distilled out of the public RSS feed for the show.
type AudioEpisode struct {
	Title       string        `json:"title,omitempty"`
	Subtitle    string        `json:"subtitle,omitempty"`
	Description string        `json:"description,omitempty"`
	PageLink    string        `json:"pageLink,omitempty"`  // URL of the landing page for this episode
	FileLink    string        `json:"fileLink,omitempty"`  // URL of the audio file for this episode
	DescLinks   []string      `json:"descLinks,omitempty"` // URLs embedded in the description
	Published   time.Time     `json:"published,omitempty"` // when this episode was published
	Duration    time.Duration `json:"duration,omitempty"`  // episode duration
	RawDesc     string        `json:"rawDescription,omitempty"`
}

func newAudioEpisode(show string, item *gofeed.Item) (*AudioEpisode, error) {
	ep := &AudioEpisode{
		Title:       item.Title,
		RawDesc:     item.Description,
		Description: item.Description,
		PageLink:    item.Link,
	}
	if ps, err := parseHTML(item.Description); err == nil {
		ep.Description = ps.Text
		ep.DescLinks = ps.Links
	}

	// The Link field may not be the actual acast page, so override it with the
	// acast extension if that is present.
	if epName := getExtensionField(item.Extensions, "acast", "episodeUrl"); epName != "" {
		ep.PageLink = "https://shows.acast.com/" + show + "/episodes/" + epName
	}

	for _, encl := range item.Enclosures {
		if encl.Type == "audio/mpeg" {
			ep.FileLink = encl.URL
			break
		}
	}

	if t := item.PublishedParsed; t != nil {
		ep.Published = *t
	}

	if ext := item.ITunesExt; ext != nil {
		ep.Subtitle = ext.Subtitle
		dur, err := parseAudioDuration(ext.Duration)
		if err == nil {
			ep.Duration = dur
		}
	}

	return ep, nil
}

var timeUnit = []time.Duration{time.Second, time.Minute, time.Hour}

func parseAudioDuration(s string) (time.Duration, error) {
	// Format: [[HH:]MM:]SS
	parts := strings.SplitN(s, ":", 3)
	// Reverse the order of parts to make parsing simpler.
	for i, j := 0, len(parts)-1; i < j; i++ {
		parts[i], parts[j] = parts[j], parts[i]
		j--
	}
	var dur time.Duration
	for i, part := range parts {
		z, err := strconv.ParseInt(part, 10, 64)
		if err != nil {
			return 0, err
		}
		dur += time.Duration(z) * timeUnit[i]
	}
	return dur, nil
}

func parseHTML(s string) (*parsedString, error) {
	tok := html.NewTokenizer(strings.NewReader(s))
	var links []string
	var buf bytes.Buffer
	var depth int
nextToken:
	for tok.Next() != html.ErrorToken {
		next := tok.Token()
		switch next.Type {
		case html.TextToken:
			buf.WriteString(next.Data)
		case html.StartTagToken:
			switch next.DataAtom {
			case atom.P:
				depth++
			case atom.A:
				href, ok := getAttr(next, "href")
				if ok {
					links = append(links, href)
				}
			case atom.Br:
				buf.WriteString("\n")
			}
		case html.EndTagToken:
			switch next.DataAtom {
			case atom.P:
				if depth > 0 {
					depth--
					if depth == 0 {
						buf.WriteString("\n")
					}
				}
			}
		case html.SelfClosingTagToken:
			// ACast inserts a disclaimer at the end after a <br/> token.
			// But it also uses breaks in the user-generated part. To tell them
			// apart, check whether we're inside a paragraph.
			if depth <= 0 {
				break nextToken
			} else {
				buf.WriteString("\n")
			}
		default:
			// discard
		}
	}
	if err := tok.Err(); err != nil && err != io.EOF {
		return nil, err
	}

	// Clean up whitespace on the ends of lines.
	var lines []string
	for _, line := range strings.Split(strings.TrimSpace(buf.String()), "\n") {
		lines = append(lines, strings.TrimSpace(line))
	}
	return &parsedString{
		Text:  strings.Join(lines, "\n"),
		Links: links,
	}, nil
}

type parsedString struct {
	Text  string
	Links []string
}

func getAttr(tok html.Token, name string) (string, bool) {
	name = strings.ToLower(name)
	for _, attr := range tok.Attr {
		if strings.ToLower(attr.Key) == name {
			return attr.Val, true
		}
	}
	return "", false
}
