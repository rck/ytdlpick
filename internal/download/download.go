// Package download runs yt-dlp for each planned item, one after another.
package download

import (
	"context"
	"fmt"
	"io"
	"os"

	"github.com/rck/ytdlpick/internal/tui"
	"github.com/rck/ytdlpick/internal/ytdlp"
)

// Result is the outcome of one download.
type Result struct {
	Item *tui.Item
	Err  error
}

// Run downloads every item serially with yt-dlp's own output on the
// terminal. It continues after failures and returns all results.
func Run(ctx context.Context, runner ytdlp.Runner, plan []*tui.Item, out io.Writer) []Result {
	var results []Result
	for i, it := range plan {
		args := append([]string{"--no-playlist"}, it.Res.Args()...)
		url := it.Info.DownloadURL()
		if url == "" {
			url = it.URL
		}
		fmt.Fprintf(out, "\n==> [%d/%d] %s\n", i+1, len(plan), it.DisplayTitle())
		fmt.Fprintf(out, "    %s %s\n", runner.Bin, shellJoin(append(append(append([]string{}, runner.Extra...), args...), "--", url)))
		cmd := runner.Command(ctx, args, url)
		cmd.Stdin = os.Stdin
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		err := cmd.Run()
		if err != nil && ctx.Err() != nil {
			results = append(results, Result{Item: it, Err: ctx.Err()})
			break
		}
		results = append(results, Result{Item: it, Err: err})
	}
	return results
}

// shellJoin quotes arguments for display.
func shellJoin(args []string) string {
	s := ""
	for i, a := range args {
		if i > 0 {
			s += " "
		}
		if needsQuote(a) {
			a = "'" + a + "'"
		}
		s += a
	}
	return s
}

func needsQuote(s string) bool {
	for _, r := range s {
		switch r {
		case ' ', '[', ']', '*', '&', '?', '$', '"', '\'', '(', ')', ';', '|', '<', '>':
			return true
		}
	}
	return s == ""
}
