# ytdlpick

A small terminal UI around [yt-dlp](https://github.com/yt-dlp/yt-dlp).
Give it URLs, pick the video, audio and subtitle tracks for each one in a
TUI, then let yt-dlp download them one after another.

```
ytdlpick [flags] URL... [-- yt-dlp arguments...]
```

Everything after `--` goes to every yt-dlp call, both the info fetches and
the downloads, for example `-- --cookies-from-browser firefox -P ~/Videos`.

Flags:

| Flag | Default | Meaning |
|---|---|---|
| `-ytdlp PATH` | `yt-dlp`, or `$YTDLP` | yt-dlp executable |
| `-jobs N` | number of CPUs, 2 to 8 | parallel info fetches |

Requirements: yt-dlp on the PATH, and ffmpeg for merging separate video
and audio streams and for embedding subtitles. Both are yt-dlp's own
requirements, nothing extra.

## Build

```
go build -o ytdlpick .
go test ./...
```

## How it works

1. Info for all URLs is fetched concurrently with `yt-dlp -J`. Playlists
   are expanded into their entries, in place. The first item is shown as
   soon as it is ready while the rest keeps loading.
2. For each item a screen with three panes appears: video, audio and
   subtitles. Defaults are yt-dlp's best video and best audio, and no
   subtitles. The audio pane offers "best audio [language]" entries when a
   video has several audio languages. Several audio tracks can be checked
   at once, for example English and German; yt-dlp then merges them into
   one file (`--audio-multistreams`). Subtitles can be downloaded as
   separate files or embedded, and automatically generated captions are
   listed with an `(auto)` marker. Long lists can be filtered with `/`.
3. Buttons:
   - **Next**: keep the choice for this item and ask for the next one.
     The next item is preselected with the same choice where it fits.
   - **Set for the rest**: apply the choice to all remaining items without
     asking. If an item cannot satisfy it, for example it has no German
     audio track, that item is shown with the parts that fit preselected.
     **Continue** there keeps applying the original choice to the items
     after it, **Set for the rest** replaces it with the new choice.
   - On the last item: **Start downloading** or **Abort**. When "set for
     the rest" resolved the tail automatically, a summary of the plan is
     shown instead, with the same two buttons.
4. The TUI exits and yt-dlp runs once per item with its normal output.
   Failures do not stop the batch. A summary is printed at the end and the
   exit status is non zero when anything failed.

Choices are carried between videos by attributes, not by yt-dlp format
IDs: audio by language (exact tag first, then the base language, so
`en-US` matches `en`), codec and container; video by resolution, frame
rate, codec and container; subtitles by language and whether they are
auto generated. "Best" always applies.

Keys: `tab`/`←`/`→` switch pane, `↑`/`↓` move, `space`/`enter` select,
`/` filter, `n` next or start, `r` set for the rest, `a` or `q` abort.

## Testing without network

`testdata/fake/yt-dlp` is a stand-in that serves the JSON fixtures in
`internal/ytdlp/testdata` and only pretends to download:

```
./ytdlpick -ytdlp testdata/fake/yt-dlp https://x/multi https://x/bbb https://x/sintel https://x/playlist
```
