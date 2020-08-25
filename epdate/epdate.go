// Program epdate checks for new episodes since the most recent
// visible on the main web site, and creates new episode files for them with
// stream URLs populated.
//
// You must provide a TWITTER_TOKEN environment variable with a Twitter API v2
// bearer token.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/inlieuoffun/tools/ilof"
)

var (
	doDryRun  = flag.Bool("dry-run", false, "Do not create or modify any files")
	doForce   = flag.Bool("force", false, "Create updates even if the files exist")
	override  = flag.String("override", "", "Override latest episode with num:date")
	checkRepo = flag.String("check-repo", "inlieuoffun.github.io",
		"Check that working directory matches this repo name")
)

const (
	episodeDir = "_episodes"
	guestFile  = "_data/guests.yaml"
)

func main() {
	flag.Parse()
	token := os.Getenv("TWITTER_TOKEN")
	if token == "" {
		log.Fatal("No TWITTER_TOKEN is set in the environment")
	}
	apiKey := os.Getenv("YOUTUBE_API_KEY")
	if apiKey == "" {
		log.Fatal("No YOUTUBE_API_KEY is set in the environment")
	}

	root, err := cdRepoRoot()
	if err != nil {
		log.Fatalf("Changing directory to repo root: %v", err)
	} else if base := filepath.Base(root); *checkRepo != "" && base != *checkRepo {
		log.Fatalf("Repository root is %q, but should be %q", base, *checkRepo)
	}

	ctx := context.Background()

	latest, err := ilof.LatestEpisode(ctx)
	if err != nil {
		log.Fatalf("Looking up latest episode: %v", err)
	}
	log.Printf("Latest episode is %s, airdate %s", latest.Episode, latest.Date)
	if *override != "" {
		ov := strings.SplitN(*override, ":", 2)
		latest.Episode = ilof.Label(ov[0])
		log.Printf(" >> override episode: %v", latest.Episode)
		if len(ov) == 2 {
			ts, _ := time.Parse("2006-01-02", ov[1])
			latest.Date = ilof.Date(ts)
			log.Printf(" >> override date: %v", latest.Date)
		}
	}

	updates, err := ilof.NewTwitter(token).Updates(ctx, latest.Date)
	if err != nil {
		log.Fatalf("Finding updates on twitter: %v", err)
	}
	log.Printf("Found %d updates on twitter since %s", len(updates), latest.Date)

	for i, up := range updates {
		epNum := latest.Episode.Int() + len(updates) - i
		epFile := fmt.Sprintf("%s-%04d.md", up.Date.Format("2006-01-02"), epNum)
		epPath := filepath.Join(episodeDir, epFile)
		exists := fileExists(epPath)

		log.Printf("Update %d: episode %d, posted %s, exists=%v",
			i+1, epNum, up.Date.Format(time.RFC822), exists)
		if exists && !*doForce {
			continue
		}
		var desc string
		if info, err := fetchEpisodeInfo(ctx, up, apiKey); err != nil {
			log.Printf("* Unable to fetch video detail from YouTube: %v", err)
		} else {
			desc = info.Description
			log.Printf("- Fetched video description from YouTube (%d bytes)", len(desc))
		}

		if *doDryRun {
			log.Printf("Not writing episode file %q, this is a dry run", epPath)
		} else if err := createEpisodeFile(epPath, epNum, desc, up); err != nil {
			log.Fatalf("Creating episode file for %d: %v", epNum, err)
		}

		for _, guest := range up.Guests {
			log.Printf("- Guest: %s", guest)
		}
		if *doDryRun {
			log.Printf("Skipped guest list update, this is a dry run")
		} else if err := ilof.AddOrUpdateGuests(epNum, guestFile, up.Guests); err != nil {
			log.Fatalf("Updating guest list: %v", err)
		}
	}
}

func createEpisodeFile(path string, num int, desc string, up *ilof.TwitterUpdate) error {
	ep, err := ilof.LoadEpisode(path)
	if err != nil {
		if !os.IsNotExist(err) {
			return err
		}
		ep = &ilof.Episode{
			Episode: ilof.Label(strconv.Itoa(num)),
			Date:    ilof.Date(up.Date),
			Detail:  desc,
		}
	}
	ep.CrowdcastURL = up.Crowdcast
	ep.YouTubeURL = up.YouTube
	return ilof.WriteEpisode(path, ep)
}

func fetchEpisodeInfo(ctx context.Context, up *ilof.TwitterUpdate, apiKey string) (*ilof.VideoInfo, error) {
	id, ok := ilof.YouTubeVideoID(up.YouTube)
	if !ok {
		return nil, errors.New("no video ID found")
	}
	return ilof.YouTubeVideoInfo(ctx, id, apiKey)
}

func cdRepoRoot() (string, error) {
	data, err := exec.Command("git", "rev-parse", "--show-toplevel").Output()
	if err != nil {
		return "", err
	}
	root := strings.TrimSpace(string(data))
	return root, os.Chdir(root)
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
