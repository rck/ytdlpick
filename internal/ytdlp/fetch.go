package ytdlp

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"
)

// Runner knows how to invoke yt-dlp.
type Runner struct {
	Bin   string   // path or name of the yt-dlp executable
	Extra []string // user supplied passthrough arguments, added to every call
}

// Command builds an exec.Cmd for yt-dlp with the passthrough arguments
// first, then ours, then "--" and the URL.
func (r Runner) Command(ctx context.Context, args []string, url string) *exec.Cmd {
	full := append([]string{}, r.Extra...)
	full = append(full, args...)
	full = append(full, "--", url)
	return exec.CommandContext(ctx, r.Bin, full...)
}

// Fetch retrieves the info for one URL. Playlists are returned flat, so
// their Entries only carry a URL and a title; the caller fetches each
// entry separately.
func (r Runner) Fetch(ctx context.Context, url string) (*Info, error) {
	cmd := r.Command(ctx, []string{"--dump-single-json", "--flat-playlist", "--no-warnings"}, url)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			return nil, errors.New(lastError(stderr.String(), err))
		}
		return nil, fmt.Errorf("run %s: %w", r.Bin, err)
	}
	info, err := Parse(bytes.TrimSpace(stdout.Bytes()))
	if err != nil {
		return nil, err
	}
	if !info.IsPlaylist() && !info.IsVideo() {
		if info.IsLive {
			return nil, errors.New("live streams are not supported")
		}
		return nil, errors.New("yt-dlp returned no downloadable formats")
	}
	return info, nil
}

// lastError extracts the most useful line from yt-dlp's stderr.
func lastError(stderr string, fallback error) string {
	lines := strings.Split(strings.TrimSpace(stderr), "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		l := strings.TrimSpace(lines[i])
		if strings.HasPrefix(l, "ERROR:") {
			return strings.TrimSpace(strings.TrimPrefix(l, "ERROR:"))
		}
	}
	for i := len(lines) - 1; i >= 0; i-- {
		if l := strings.TrimSpace(lines[i]); l != "" {
			return l
		}
	}
	return fallback.Error()
}

// Entry is one element of a flat playlist.
type Entry struct {
	URL   string
	Title string
	Info  *Info // set when the playlist already delivered full information
}

// PlaylistEntries decodes the entries of a flat playlist.
func (i *Info) PlaylistEntries() ([]Entry, error) {
	var entries []Entry
	for _, raw := range i.Entries {
		if len(raw) == 0 || string(raw) == "null" {
			continue // unavailable entry
		}
		e, err := Parse(raw)
		if err != nil {
			return nil, err
		}
		url := e.URL
		if url == "" {
			url = e.WebpageURL
		}
		if url == "" {
			continue
		}
		entry := Entry{URL: url, Title: e.Title}
		if e.IsVideo() {
			entry.Info = e
			if e.WebpageURL == "" {
				e.WebpageURL = url
			}
		}
		entries = append(entries, entry)
	}
	return entries, nil
}
