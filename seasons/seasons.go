// Program season updates episode files to add a "season" value.
// By fiat Ben, each season is 250 episodes.
package main

import (
	"flag"
	"log"
	"os"
	"path/filepath"
	"regexp"

	"github.com/inlieuoffun/tools/ilof"
)

var (
	epDir = flag.String("dir", "", "Episodes directory")

	epFile = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}-[-\w]+\.md$`)
)

func main() {
	flag.Parse()
	if *epDir == "" {
		log.Fatal("You must provide a non-empty episodes -dir")
	}
	elts, err := os.ReadDir(*epDir)
	if err != nil {
		log.Fatalf("ReadDir: %v", err)
	}
	for _, elt := range elts {
		if !elt.Type().IsRegular() {
			continue
		}
		name := elt.Name()
		if !epFile.MatchString(name) {
			log.Printf("Skip %q", name)
			continue
		}
		path := filepath.Join(*epDir, name)
		ep, err := ilof.LoadEpisode(path)
		if err != nil {
			log.Fatalf("LoadEpisode: %v", err)
		}
		if ep.Season != 0 {
			log.Printf("Episode %q already has season %d (skipped)", ep.Episode, ep.Season)
			continue
		}

		ep.Season = int(ep.Episode.Number()/250) + 1
		log.Printf("Assigned episode %q to season %d", ep.Episode, ep.Season)
		if err := ilof.WriteEpisode(path, ep); err != nil {
			log.Fatalf("WriteEpisode: %v", err)
		}
	}
}
