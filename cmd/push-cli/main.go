// Command push-cli pushes files with a .picpress-config, headless: the same
// engine, credentials exchange and manifests as the app (internal/push), for
// testing a real backend end to end without the GUI.
//
//	go run ./cmd/push-cli -config MZ-901.picpress-config photo.jpg notes.md
//
// Images are compressed to WebP (quality 80) as the app's default preset;
// documents go byte for byte; videos are skipped (no ffmpeg here). The
// journal lives in a temporary directory, so every run is a new batch.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"time"

	"picpress/internal/imagepress"
	"picpress/internal/push"
)

func main() {
	cfgPath := flag.String("config", "", "the .picpress-config to push with")
	flag.Parse()
	if *cfgPath == "" || flag.NArg() == 0 {
		fmt.Fprintln(os.Stderr, "usage: push-cli -config <file> <paths…>")
		os.Exit(2)
	}
	data, err := os.ReadFile(*cfgPath)
	if err != nil {
		fail(err)
	}
	cfg, err := push.ParseConfig(data)
	if err != nil {
		fail(err)
	}
	dir, err := os.MkdirTemp("", "push-cli-")
	if err != nil {
		fail(err)
	}
	defer os.RemoveAll(dir)
	journal, err := push.OpenJournal(dir)
	if err != nil {
		fail(err)
	}
	defer journal.Close()

	paths := make([]string, 0, flag.NArg())
	for _, p := range flag.Args() {
		abs, err := filepath.Abs(p)
		if err != nil {
			fail(err)
		}
		paths = append(paths, abs)
	}
	device := push.DeviceInfo{DeviceID: "push-cli", AppVersion: "push-cli", OS: runtime.GOOS + "-" + runtime.GOARCH}
	engine := &push.Engine{
		S3:       push.NewS3(),
		Session:  &push.Session{Source: push.TokenSource{URL: cfg.CredentialsURL, Token: cfg.Token, Device: device}},
		Journal:  journal,
		StageDir: filepath.Join(dir, "stage"),
		Producers: map[push.Kind]push.Producer{
			push.KindImage:    push.ImageProducer{Options: imagepress.Options{Format: imagepress.FormatWebP, Quality: 80}},
			push.KindDocument: push.DocumentProducer{},
		},
		DeviceID: device.DeviceID, AppVersion: device.AppVersion,
		Probe: func(context.Context) bool { return true },
		OnFile: func(u push.FileUpdate) {
			if u.Status == push.StatusUploaded || u.Status == push.StatusFailed || u.Status == push.StatusDuplicate {
				fmt.Printf("%-18s %s → %s\n", u.Status, u.Source, u.Key)
			}
		},
	}
	plan := push.Scan(paths, push.PlanOptions{Limits: cfg.Limits, Supported: map[push.Kind]bool{push.KindImage: true, push.KindDocument: true}})
	for _, s := range plan.Skipped {
		fmt.Printf("%-18s %s (%s)\n", "skipped", s.Path, s.Reason)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	batch := push.BatchState{ID: fmt.Sprintf("cli-%d", time.Now().Unix()), Paths: paths}
	res, err := engine.Run(ctx, batch, plan, plan.Entries)
	out, _ := json.MarshalIndent(res, "", "  ")
	fmt.Println(string(out))
	if err != nil {
		fail(err)
	}
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, "push-cli:", err)
	os.Exit(1)
}
