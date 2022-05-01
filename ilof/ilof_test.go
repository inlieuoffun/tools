// Copyright (C) 2020 Michael J. Fromberger. All Rights Reserved.

package ilof_test

import (
	"context"
	"flag"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/inlieuoffun/tools/ilof"
)

var doManual = flag.Bool("manual", false, "Run manual tests")

func TestYouTubeVideoID(t *testing.T) {
	tests := []struct {
		input, wantID string
		wantOK        bool
	}{
		{"https://google.com", "", false},
		{"http://youtu.be/foobar?q=baz", "foobar", true},
		{"https://www.youtube.com/watch?v=kiss_me", "kiss_me", true},
		{"https://youtube.com/watch?v=you_fool", "you_fool", true},
		{"https://youtube.com/watch?v=you_fool&feature=youtu.be", "you_fool", true},
		{"https://youtube.com/watch", "", false},
		{"https://youtu.be/", "", false},
	}
	for _, test := range tests {
		id, ok := ilof.YouTubeVideoID(test.input)
		if id != test.wantID {
			t.Errorf("YouTubeVideoID(%q): got ID %q, want %q", test.input, id, test.wantID)
		}
		if ok != test.wantOK {
			t.Errorf("YouTubeVideoID(%q): got %v, want %v", test.input, ok, test.wantOK)
		}
	}
}

func TestVideoInfo(t *testing.T) {
	if !*doManual {
		t.Skip("Skipping manual test (-manual=false)")
	}
	apiKey := os.Getenv("YOUTUBE_API_KEY")
	if apiKey == "" {
		t.Fatal("No YOUTUBE_API_KEY is set")
	}

	ctx := context.Background()
	info, err := ilof.YouTubeVideoInfo(ctx, "nphZCMuhgUU", apiKey)
	if err != nil {
		t.Fatalf("YouTubeVideoInfo failed: %v", err)
	}
	t.Logf("Video %q description:\n>> %s", info.ID, info.Description)
}

func TestVideoTranscript(t *testing.T) {
	if !*doManual {
		t.Skip("Skipping manual test (-manual=false)")
	}

	//const videoID = "8qvF9EtNdUE"
	const videoID = "s9vNrZSRUbc"
	ctx := context.Background()
	url, err := ilof.YouTubeCaptionURL(ctx, videoID)
	if err != nil {
		t.Fatalf("Fetching caption URL for %q failed: %v", videoID, err)
	} else if url == "" {
		t.Fatalf("No caption found for %q", videoID)
	}
	t.Logf("Caption URL for %q is %s", videoID, url)

	cap, err := ilof.YouTubeCaptionData(ctx, url)
	if err != nil {
		t.Fatalf("Fetching caption data: %v", err)
	}

	for i, elt := range cap.Captions {
		at := time.Duration(elt.Start) * time.Second
		t.Logf("[%d]: %v\t%s", i+1, at, elt.Text)
	}
}

func TestLatestEpisode(t *testing.T) {
	if !*doManual {
		t.Skip("Skipping manual test (-manual=false)")
	}
	token := os.Getenv("TWITTER_TOKEN")
	if token == "" {
		t.Fatal("No TWITTER_TOKEN is set")
	}

	ctx := context.Background()
	ep, err := ilof.LatestEpisode(ctx)
	if err != nil {
		t.Fatalf("LatestEpisode failed: %v", err)
	}
	t.Logf(`Latest episode %s:
Date:      %s
YouTube:   %s
Crowdcast: %s
Guests:    %+v
Summary:
> %s`, ep.Episode, ep.Date, ep.YouTubeURL, ep.CrowdcastURL,
		strings.Join(ep.Guests, ", "), ep.Summary)

	ups, err := ilof.TwitterUpdates(ctx, token, ep.Date)
	if err != nil {
		t.Fatalf("TwitterUpdates failed: %v", err)
	}

	for i, up := range ups {
		num := int(ep.Episode.Number()) + len(ups) - i
		t.Logf(`Probable episode %d:
Date:      %s
YouTube:   %s
Crowdcast: %s
Guests:    %+v`, num, up.Date.Format("2006-01-02"), up.YouTube, up.Crowdcast, up.Guests)
	}
}

func TestAcastFeed(t *testing.T) {
	if !*doManual {
		t.Skip("Skipping manual test (-manual=false)")
	}
	eps, err := ilof.LoadAcastFeed(context.Background(), ilof.AcastFeedURL)
	if err != nil {
		t.Fatalf("LoadAcastFeed: %v", err)
	}
	for i, ep := range eps {
		t.Logf(`Entry %d:
Title:     %s
Subtitle:  %s
PageLink:  %s
FileLink:  %s
Duration:  %v
Published: %v
`, i+1, ep.Title, ep.Subtitle, ep.PageLink, ep.FileLink, ep.Duration, ep.Published)
	}
}

func TestSimilarity(t *testing.T) {
	tests := []struct {
		a, b string
		want float64
	}{
		{"", "", 1},
		{"xyz", "", 0},
		{"", "xyz", 0},
		{"a b c", "d e f", 0},
		{"xyz", "xyz", 1},
		{"a b c", "c b a", 1},
		{"a b c", "b d f", 1. / 3},
		{"a b", "b c", 0.5},

		{"you are everything that is wrong with the world",
			"you are everything wrong", 2. / 3},
	}
	for _, test := range tests {
		got := ilof.Similarity(test.a, test.b)
		if got != test.want {
			t.Errorf("Similarity(%q, %q): got %v, want %v", test.a, test.b, got, test.want)
		}
	}
}
