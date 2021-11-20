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
	"flag"
	"fmt"
	"log"

	"github.com/inlieuoffun/tools/ilof"
)

func main() {
	flag.Parse()

	ctx := context.Background()
	audio, err := ilof.LoadAcastFeed(ctx, ilof.AcastFeedURL)
	if err != nil {
		log.Fatalf("Loading acast feed: %v", err)
	}
	log.Printf("Loaded %d audio episodes", len(audio))

	eps, err := ilof.AllEpisodes(ctx)
	if err != nil {
		log.Fatalf("Loading ILoF episodes: %v", err)
	}
	log.Printf("Loaded %d ILoF episodes", len(eps))

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

	for _, ep := range audio {
		if _, ok := acastIndex[ep.PageLink]; !ok {
			continue // already recorded
		}
		fmt.Printf("%s %q\n\t%s\n", ep.Published.Format("2006-01-02 15:04"),
			ep.Title, ep.PageLink)
	}
}
