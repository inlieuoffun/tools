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
	videoID = flag.String("id", "", "Video ID to fetch (required")
)

func main() {
	flag.Parse()
	switch {
	case *videoID == "":
		log.Fatal("You must set a non-empty video -id")
	}

	ctx := context.Background()
	url, err := ilof.YouTubeCaptionURL(ctx, *videoID)
	if err != nil {
		log.Fatalf("Getting caption URL: %v", err)
	} else if url == "" {
		log.Fatalf("No caption URL found for video ID %q", *videoID)
	}

	cap, err := ilof.YouTubeCaptionData(ctx, url)
	if err != nil {
		log.Fatalf("Getting caption data: %v", err)
	}
	cap.VideoID = *videoID

	// TODO(creachadair): Other output formats.
	bits, err := json.Marshal(struct {
		Transcript *ilof.Transcript `json:"transcript"`
	}{cap})
	if err != nil {
		log.Fatalf("Encoding output: %v", err)
	}
	fmt.Println(string(bits))
}
