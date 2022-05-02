// Program fytt fetches YouTube text transcripts.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"

	"github.com/inlieuoffun/tools/ilof"
)

var (
	videoID = flag.String("id", "", "Video ID to fetch")
	episode = flag.String("episode", "", "Episode number")
)

func init() {
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, `Usage: %[1]s -id <video-id>
       %[1]s -episode <episode-id>

Fetch text captions for a YouTube video. Either the -id of the video
must be specified directly, or the -episode whose video URL is to be
fetched.

Output is written to stdout as JSON:

  {
    "transcript": {
      "videoID": "<video-id>",
      "captionsURL": "<captions-url>",
      "captions": [{
         "startSec": 123.4,
         "durationSec": 5.6,
         "text": "... text of transcription segment ..."
      }, ...]
    }
  }

Options:
`, filepath.Base(os.Args[0]))
		flag.PrintDefaults()
	}
}

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
