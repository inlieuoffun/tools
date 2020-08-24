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

func TestLatestEpisode(t *testing.T) {
	if !*doManual {
		t.Skip("Skipping manual test (-manual=false)")
	}
	token := os.Getenv("TWITTER_TOKEN")
	if token == "" {
		t.Fatal("No TWITTER_TOKEN is set")
	}
	cli := ilof.NewTwitter(token)

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

	ups, err := cli.Updates(ctx, ep.Date)
	if err != nil {
		t.Fatalf("TwitterUpdates failed: %v", err)
	}

	for i, up := range ups {
		num := ep.Episode.Int() + len(ups) - i
		t.Logf(`Probable episode %d:
Date:      %s
YouTube:   %s
Crowdcast: %s
Guests:    %+v`, num, up.Date.Format("2006-01-02"), up.YouTube, up.Crowdcast, up.Guests)
	}
}
