// Program fytt fetches YouTube text transcripts.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"

	"github.com/inlieuoffun/tools/ilof"
)

var (
	videoID = flag.String("id", "", "Video ID to fetch")
	episode = flag.String("episode", "", "Episode number")
)

func main() {
	flag.Parse()
	if *videoID == "" && *episode == "" {
		log.Fatal("You must set a non-empty video -id or an -episode")
	}

	ctx := context.Background()
	if *episode != "" {
		ep, err := ilof.FetchEpisode(ctx, *episode)
		if err != nil {
			log.Fatalf("Fetching episode %q: %v", *episode, err)
		}
		id, ok := ilof.YouTubeVideoID(ep.YouTubeURL)
		if !ok {
			log.Fatalf("Unable to find video ID for episode %q", ep.Episode)
		}
		log.Printf("Found video ID %q for episode %q", id, ep.Episode)
		*videoID = id
	}

	url, err := ilof.YouTubeCaptionURL(ctx, *videoID)
	if err != nil {
		log.Fatalf("Getting caption URL: %v", err)
	} else if url == "" {
		log.Fatalf("No caption URL found for video ID %q", *videoID)
	}
	log.Printf("Caption URL: %q", url)

	cap, err := ilof.YouTubeCaptionData(ctx, url)
	if err != nil {
		log.Fatalf("Getting caption data: %v", err)
	}
	cap.VideoID = *videoID
	log.Printf("Found %d captions for ID %q", len(cap.Captions), cap.VideoID)

	// TODO(creachadair): Other output formats.
	bits, err := json.Marshal(struct {
		Transcript *ilof.Transcript `json:"transcript"`
	}{cap})
	if err != nil {
		log.Fatalf("Encoding output: %v", err)
	}
	fmt.Println(string(bits))
}
