// Program scancast looks for audio episodes in the acast RSS feed that may not
// have yet been recorded in the episode log.
//
// Ideally we would automatically correlate these, but the date of publication
// is different, and the episode numbers on acast are hand-assigned and usually
// wrong. So instead, we list all the known audio episodes, cross off the ones
// that have already been recorded, and list the leftovers.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"

	"github.com/inlieuoffun/tools/ilof"
)

var (
	doFeed    = flag.Bool("json-feed", false, "Print Acast feed as JSON and exit")
	doMissing = flag.Bool("log-missing", false, "Log episodes missing audio and exit")
)

func main() {
	flag.Parse()

	ctx := context.Background()
	audio, err := ilof.LoadAcastFeed(ctx, ilof.AcastFeedURL)
	if err != nil {
		log.Fatalf("Loading acast feed: %v", err)
	}
	log.Printf("Loaded %d audio episodes", len(audio))
	if *doFeed {
		mustWriteJSON(struct {
			E []*ilof.AudioEpisode `json:"episodes"`
		}{E: audio})
		return
	}

	eps, err := ilof.AllEpisodes(ctx)
	if err != nil {
		log.Fatalf("Loading ILoF episodes: %v", err)
	}
	log.Printf("Loaded %d ILoF episodes", len(eps))

	if *doMissing {
		var missing []*ilof.Episode
		for _, ep := range eps {
			if ep.AcastURL == "" {
				missing = append(missing, ep)
			}
		}
		mustWriteJSON(struct {
			M []*ilof.Episode `json:"missing"`
		}{M: missing})
		return
	}

	// Episodes that have been updated with acast links have the landing page
	// link in their AcastURL field. Prune any audio episodes that have already
	// been recorded and report on the rest.
	acastIndex := make(map[string]*ilof.AudioEpisode)
	for _, ep := range audio {
		acastIndex[ep.PageLink] = ep
	}
	for _, ep := range eps {
		delete(acastIndex, ep.AcastURL)
	}
	if len(acastIndex) == 0 {
		log.Fatal("No audio episodes require updating")
	}

	for _, ep := range audio {
		if _, ok := acastIndex[ep.PageLink]; !ok {
			continue // already recorded
		}
		log.Printf("%s %q", ep.Published.Format("2006-01-02 15:04"), ep.Title)
		fmt.Printf("acast: %s\n", ep.PageLink)
		if ep.FileLink != "" {
			fmt.Printf("audio-file: %s\n", ep.FileLink)
		}
	}
}

func mustWriteJSON(v interface{}) {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	if err := enc.Encode(v); err != nil {
		log.Fatalf("Encoding JSON: %v", err)
	}
}
