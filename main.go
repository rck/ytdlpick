// Command ytdlpick is a terminal UI around yt-dlp: it fetches the
// available video, audio and subtitle tracks for each URL, lets you pick
// what to download and then runs yt-dlp for every item.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"runtime"
	"strings"
	"syscall"

	"github.com/rck/ytdlpick/internal/download"
	"github.com/rck/ytdlpick/internal/tui"
	"github.com/rck/ytdlpick/internal/ytdlp"
)

func usage() {
	fmt.Fprintf(os.Stderr, `Usage: %s [flags] URL... [-- yt-dlp arguments...]

Interactively choose video, audio and subtitle tracks for each URL, then
download them with yt-dlp. Playlists are expanded into their entries.
Arguments after "--" are passed to every yt-dlp invocation, for example
"-- --cookies-from-browser firefox -P ~/Videos".

Flags:
`, os.Args[0])
	flag.PrintDefaults()
}

func main() {
	os.Exit(run())
}

func run() int {
	var ours []string
	var extra []string
	for i, a := range os.Args[1:] {
		if a == "--" {
			ours = os.Args[1 : i+1]
			extra = os.Args[i+2:]
			break
		}
	}
	if ours == nil {
		ours = os.Args[1:]
	}

	fs := flag.NewFlagSet(os.Args[0], flag.ExitOnError)
	fs.Usage = usage
	bin := fs.String("ytdlp", envOr("YTDLP", "yt-dlp"), "yt-dlp executable (env YTDLP)")
	jobs := fs.Int("jobs", max(2, min(8, runtime.NumCPU())), "parallel yt-dlp info fetches")
	if err := fs.Parse(ours); err != nil {
		return 2
	}
	urls := fs.Args()
	if len(urls) == 0 {
		usage()
		return 2
	}
	if _, err := exec.LookPath(*bin); err != nil {
		fmt.Fprintf(os.Stderr, "yt-dlp not found: %v\n", err)
		return 1
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	runner := ytdlp.Runner{Bin: *bin, Extra: extra}
	plan, aborted, err := tui.Run(ctx, runner, urls, *jobs)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		return 1
	}
	if aborted {
		fmt.Fprintln(os.Stderr, "aborted, nothing downloaded")
		return 130
	}
	if len(plan) == 0 {
		fmt.Fprintln(os.Stderr, "nothing to download")
		return 1
	}

	fmt.Printf("Downloading %d item(s):\n", len(plan))
	for i, it := range plan {
		fmt.Printf("  %2d. %s\n      video: %s · audio: %s · subtitles: %s\n",
			i+1, it.DisplayTitle(), it.Res.VideoText(), it.Res.AudioText(), it.Res.SubsText())
	}

	results := download.Run(ctx, runner, plan, os.Stdout)

	failed := 0
	fmt.Println()
	fmt.Println(strings.Repeat("=", 60))
	for _, r := range results {
		if r.Err != nil {
			failed++
			fmt.Printf("FAILED  %s: %v\n", r.Item.DisplayTitle(), r.Err)
		} else {
			fmt.Printf("ok      %s\n", r.Item.DisplayTitle())
		}
	}
	if missing := len(plan) - len(results); missing > 0 {
		fmt.Printf("%d item(s) not attempted\n", missing)
		failed += missing
	}
	fmt.Printf("%d of %d succeeded\n", len(results)-countFailed(results), len(plan))
	if failed > 0 {
		return 1
	}
	return 0
}

func countFailed(rs []download.Result) int {
	n := 0
	for _, r := range rs {
		if r.Err != nil {
			n++
		}
	}
	return n
}

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
