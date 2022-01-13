// Copyright (C) 2020 Michael J. Fromberger. All Rights Reserved.

// Program epdate checks for new episodes since the most recent
// visible on the main web site, and creates new episode files for them with
// stream URLs populated.
//
// You must provide a TWITTER_TOKEN environment variable with a Twitter API v2
// bearer token.
//
// Exit status 0 means an update was generated.
// Exit status 3 means no update was available.
// Any other status means some other failure.
package main

import (
	"bytes"
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
	"github.com/inlieuoffun/tools/repo"
)

var (
	doDryRun   = flag.Bool("dry-run", false, "Do not create or modify any files")
	doForce    = flag.Bool("force", false, "Create updates even if the files exist")
	doEdit     = flag.Bool("edit", false, "Edit new or modified files after update")
	doPoll     = flag.Bool("poll", false, "Poll for updates")
	doPollOne  = flag.Bool("poll-one", false, "Poll for a single update")
	checkVideo = flag.Bool("check-video", true, "Fail if no video ID is found")
	override   = flag.String("override", "", "Override latest episode with num:date")
	checkRepo  = flag.String("check-repo", "inlieuoffun.github.io",
		"Check that working directory matches this repo name")

	// The error reported when a video ID is not found in the description.
	errNoVideoID = errors.New("no video ID found")
)

const (
	episodeDir = "_episodes"
	guestFile  = "_data/guests.yaml"
)

func main() {
	flag.Parse()
	token := os.Getenv("TWITTER_TOKEN")
	if token == "" {
		log.Fatal(`No TWITTER_TOKEN is set in the environment.
  If you need a token, visit https://developer.twitter.com/en/portal/dashboard`)
	}
	apiKey := os.Getenv("YOUTUBE_API_KEY")
	if apiKey == "" {
		log.Fatal(`No YOUTUBE_API_KEY is set in the environment.
  If you need a key, visit https://console.developers.google.com/apis/credentials`)
	}

	if err := repo.ChdirRoot(); err != nil {
		log.Fatalf("Changing directory to repo root: %v\n(This tool requires a repository clone)", err)
	}
	if *checkRepo != "" {
		remote, err := repo.RemoteRepo("origin")
		if err != nil {
			log.Fatalf("Finding origin URL: %v", err)
		} else if remote != *checkRepo {
			log.Fatalf("Remote is %q, but should be %q", remote, *checkRepo)
		}
	}

	ctx := context.Background()
	for {
		latestDate, didUpdate := checkForUpdate(ctx, token, apiKey)
		if didUpdate {
			if *doPollOne || !*doPoll {
				return
			}
		} else if !*doPoll && !*doPollOne {
			os.Exit(3)
		}

		now := time.Now()
		start := todayStart(now)
		if isSameDate(time.Time(latestDate), now) {
			nextWake := nextStartAfter(now).Add(-8 * time.Hour)
			diff := nextWake.Sub(now)
			log.Printf("Next episode is on %s; sleeping for %v (until %s)...",
				nextWake.Format("2006-01-02"), diff.Round(1*time.Minute),
				nextWake.In(time.Local).Format(time.Kitchen))
			time.Sleep(diff)
			continue
		}

		diff := start.Sub(now)
		wait := diff / 3
		if wait < 1*time.Minute {
			wait = 1 * time.Minute
		}
		log.Printf("Next episode in %v; sleeping for %v...",
			diff.Round(1*time.Minute), wait.Round(1*time.Minute))
		time.Sleep(wait)
	}
}

func checkForUpdate(ctx context.Context, token, apiKey string) (ilof.Date, bool) {
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

	updates, err := ilof.TwitterUpdates(ctx, token, latest.Date)
	if err != nil {
		log.Printf("Finding updates on twitter: %v", err)
		if err == ilof.ErrNoUpdates {
			return latest.Date, false
		}
		os.Exit(1)
	}
	log.Printf("Found %d updates on twitter since %s", len(updates), latest.Date)

	var editPaths []string
	var guestsDirty bool

	numValid := 0
	for i, up := range updates {
		epNum := int(latest.Episode.Number()) + numValid + 1
		epFile := fmt.Sprintf("%s-%04d.md", up.AirDate.Format("2006-01-02"), epNum)
		epPath := filepath.Join(episodeDir, epFile)
		exists := fileExists(epPath)

		log.Printf("Update %d: episode %d, posted %s, air %s, exists=%v",
			i+1, epNum, up.Date.In(time.Local).Format(time.RFC822),
			up.AirDate.In(time.Local).Format("2006-01-02"), exists)
		if exists && !*doForce {
			continue
		}
		var desc string
		if info, err := fetchEpisodeInfo(ctx, up, apiKey); err == errNoVideoID {
			if *checkVideo {
				log.Print("* No video ID found for this episode; skipping")
				continue
			}
		} else if err != nil {
			log.Printf("* Unable to fetch video detail from YouTube: %v", err)
		} else {
			desc = info.Description
			log.Printf("- Fetched video description from YouTube (%d bytes)", len(desc))
		}

		if *doDryRun {
			log.Printf("@ Not writing episode file %q, this is a dry run", epPath)
		} else if err := createEpisodeFile(epPath, epNum, desc, up); err != nil {
			log.Fatalf("* Creating episode file for %d: %v", epNum, err)
		} else {
			log.Printf("- Wrote episode %d file: %s", epNum, epPath)
		}

		for _, guest := range up.Guests {
			log.Printf("- Guest: %s", guest)
		}
		if *doDryRun {
			log.Printf("@ Skipped guest list update, this is a dry run")
		} else if err := ilof.AddOrUpdateGuests(float64(epNum), guestFile, up.Guests); err != nil {
			log.Fatalf("* Updating guest list: %v", err)
		}
		editPaths = append(editPaths, epPath)
		guestsDirty = guestsDirty || len(up.Guests) != 0
		numValid++
	}
	if guestsDirty {
		editPaths = append(editPaths, guestFile)
	}

	if *doEdit && len(editPaths) != 0 {
		if err := editFiles(editPaths); err != nil {
			log.Fatalf("Edit failed: %v", err)
		}
	}
	return latest.Date, true
}

func createEpisodeFile(path string, num int, desc string, up *ilof.TwitterUpdate) error {
	ep, err := ilof.LoadEpisode(path)
	if err != nil {
		if !os.IsNotExist(err) {
			return err
		}
		ep = &ilof.Episode{
			Episode: ilof.Label(strconv.Itoa(num)),
			Date:    ilof.Date(up.AirDate),
			Season:  int(float64(num)/250) + 1,
			Detail:  desc,
		}
	}
	if ilof.Similarity(desc, "cheese night") != 0 {
		ep.AddTag("cheese-night")
	}
	ep.CrowdcastURL = up.Crowdcast
	ep.YouTubeURL = up.YouTube
	return ilof.WriteEpisode(path, ep)
}

func fetchEpisodeInfo(ctx context.Context, up *ilof.TwitterUpdate, apiKey string) (*ilof.VideoInfo, error) {
	id, ok := ilof.YouTubeVideoID(up.YouTube)
	if !ok {
		return nil, errNoVideoID
	}
	return ilof.YouTubeVideoInfo(ctx, id, apiKey)
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func editFiles(paths []string) error {
	editor := os.Getenv("EDITOR")
	if editor == "" {
		return errors.New("no EDITOR is defined")
	}

	// Ensure the editor can interact with the controlling terminal.
	f, err := os.Open("/dev/tty")
	if err != nil {
		return err
	}
	defer f.Close()

	cmd := exec.Command(editor, paths...)
	buf := bytes.NewBuffer(nil)
	cmd.Stdin = f
	cmd.Stdout = f
	cmd.Stderr = buf
	err = cmd.Run()
	if err != nil {
		msg := buf.String()
		if msg != "" {
			return errors.New(msg)
		}
		return err
	}
	return nil
}

func todayStart(now time.Time) time.Time {
	day := now.Day()
	wd := now.Weekday()
	if wd%2 == 0 {
		day++
	}
	if wd == time.Saturday {
		day++ // advance past Sunday
	}
	// N.B. we rely on the fact that Date normalizes days out of range.
	return time.Date(now.Year(), now.Month(), day, 22, 0, 0, 0, time.UTC)
}

func nextStartAfter(now time.Time) time.Time {
	next := time.Date(now.Year(), now.Month(), now.Day()+1, 0, 0, 0, 1, time.UTC)
	return todayStart(next)
}

func isSameDate(now, then time.Time) bool {
	return now.Year() == then.Year() && now.Month() == then.Month() && now.Day() == then.Day()
}
