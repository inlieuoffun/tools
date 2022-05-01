package ilof

import (
	"bytes"
	"context"
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"html"
	"net/http"
)

// youTubeWatchBase is the base URL for the "watch" page for a video ID.
const youTubeWatchBase = `https://www.youtube.com/watch?v=%s`

func loadWatchPage(ctx context.Context, id string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", fmt.Sprintf(youTubeWatchBase, id), nil)
	if err != nil {
		return nil, err
	}
	return loadRequest(ctx, req)
}

// YouTubeCaptionURL returns the URL of the captions for the specified video
// ID.  It returns "" without error if the video exists but lacks captions.
func YouTubeCaptionURL(ctx context.Context, id string) (string, error) {
	bits, err := loadWatchPage(ctx, id)
	if err != nil {
		return "", err
	}
	const needle = `"captions":`
	i := bytes.Index(bits, []byte(needle))
	if i < 0 {
		if bytes.Contains(bits, []byte(`class="g-recaptcha"`)) {
			return "", errors.New("rate limit exceeded")
		} else if !bytes.Contains(bits, []byte(`playabilityStatus`)) {
			return "", fmt.Errorf("video ID %q not found", id)
		}
		return "", nil
	}

	var data struct {
		R *struct {
			C []*captionInfo `json:"captionTracks"`
		} `json:"playerCaptionsTracklistRenderer"`
	}

	// Decode the JSON blob. Use a json.Decoder so that the garbage in the file
	// after the blob we're interested in can be ignored.
	dec := json.NewDecoder(bytes.NewReader(bits[i+len(needle):]))
	if err := dec.Decode(&data); err != nil {
		return "", err
	}

	if data.R == nil && len(data.R.C) == 0 {
		return "", nil
	}

	// Look for an English transcription, if available.
	for _, info := range data.R.C {
		if info.Lang == "en" {
			return info.URL, nil
		}
	}

	// If we don't find English specifically, just take the first one.
	return data.R.C[0].URL, nil
}

type captionInfo struct {
	URL  string `json:"baseUrl"`
	Lang string `json:"languageCode"`
	Kind string `json:"kind"`

	// other fields ignored
}

func loadCaptionXML(ctx context.Context, url string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, err
	}
	return loadRequest(ctx, req)
}

// YouTubeCaptionData loads and parses the specified caption URL and returns
// the resulting caption.
func YouTubeCaptionData(ctx context.Context, url string) (*Caption, error) {
	bits, err := loadCaptionXML(ctx, url)
	if err != nil {
		return nil, err
	}

	cap := new(Caption)
	dec := xml.NewDecoder(bytes.NewReader(bits))
	dec.Entity = xml.HTMLEntity
	if err := dec.Decode(cap); err != nil {
		return nil, fmt.Errorf("decoding XML: %w", err)
	}
	for i := range cap.Texts {
		decodeEntities(&cap.Texts[i].Text)
	}
	return cap, nil
}

type Caption struct {
	XMLName xml.Name      `xml:"transcript"`
	Texts   []CaptionText `xml:"text"`
}

// <text start="3285.28" dur="4.88">surprised you with how they comport</text>
type CaptionText struct {
	Start    float64 `xml:"start,attr"`
	Duration float64 `xml:"dur,attr"`
	Text     string  `xml:",chardata"`
}

func decodeEntities(s *string) { *s = html.UnescapeString(*s) }
