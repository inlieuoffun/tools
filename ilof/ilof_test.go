// Copyright (C) 2020 Michael J. Fromberger. All Rights Reserved.

package ilof_test

import (
	"context"
	"flag"
	"os"
	"strings"
	"testing"

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
