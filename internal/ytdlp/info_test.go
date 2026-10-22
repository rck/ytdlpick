package ytdlp

import (
	"os"
	"path/filepath"
	"testing"
)

func load(t *testing.T, name string) *Info {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatal(err)
	}
	info, err := Parse(data)
	if err != nil {
		t.Fatal(err)
	}
	return info
}

func TestParseVideo(t *testing.T) {
	info := load(t, "video_bbb.json")
	if info.IsPlaylist() || !info.IsVideo() {
		t.Fatalf("expected a video, got type %q", info.Type)
	}
	for _, f := range info.Video {
		if f.Kind() != KindVideo && f.Kind() != KindMuxed {
			t.Errorf("video list contains %s of kind %d", f.ID, f.Kind())
		}
		if f.Ext == "mhtml" {
			t.Errorf("storyboard %s not filtered", f.ID)
		}
	}
	for _, f := range info.Audio {
		if f.Kind() != KindAudio {
			t.Errorf("audio list contains %s of kind %d", f.ID, f.Kind())
		}
	}
	// HLS duplicates are removed: 1080p60 vp9 mp4 exists as 617 (m3u8)
	// only, 1080p60 vp9 webm as 303 (https) only.
	seen := map[string]int{}
	for _, f := range info.Video {
		seen[VideoLabel(f)]++
	}
	for l, n := range seen {
		if n > 1 {
			t.Errorf("duplicate video label %q (%d)", l, n)
		}
	}
	if info.Video[0].Height != 2160 {
		t.Errorf("best first: got %dp", info.Video[0].Height)
	}
	if got := VideoLabel(info.Video[0]); got != "2160p60 vp9 webm 1.3GiB" {
		t.Errorf("label = %q", got)
	}
	if info.MuxedOnly() {
		t.Error("bbb has audio-only formats")
	}
}

func TestParseMultiAudio(t *testing.T) {
	info := load(t, "video_multiaudio.json")
	langs := info.AudioLanguages()
	if len(langs) < 20 || langs[0] != "en-US" {
		t.Fatalf("languages = %v", langs)
	}
	if !info.Audio[0].Original() {
		t.Errorf("first audio should be the original track: %+v", info.Audio[0])
	}
	if len(info.Subs) == 0 {
		t.Fatal("no subtitles")
	}
	manual, auto := 0, 0
	for _, s := range info.Subs {
		if s.Auto {
			auto++
		} else {
			manual++
			if auto > 0 {
				t.Error("manual subtitles must come before auto captions")
			}
		}
	}
	if manual != 17 || auto != 157 {
		t.Errorf("manual=%d auto=%d", manual, auto)
	}
}

func TestParsePlaylist(t *testing.T) {
	info := load(t, "playlist_flat.json")
	if !info.IsPlaylist() {
		t.Fatal("expected playlist")
	}
	entries, err := info.PlaylistEntries()
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 4 {
		t.Fatalf("entries = %d", len(entries))
	}
	if entries[0].URL == "" || entries[0].Title == "" || entries[0].Info != nil {
		t.Errorf("flat entry = %+v", entries[0])
	}
}

func TestHelpers(t *testing.T) {
	cases := map[string]string{"avc1.64002A": "avc1", "vp09.00.41.08": "vp9", "mp4a.40.2": "aac", "opus": "opus", "none": "none", "": ""}
	for in, want := range cases {
		if got := CodecFamily(in); got != want {
			t.Errorf("CodecFamily(%q) = %q, want %q", in, got, want)
		}
	}
	for in, want := range map[string]string{"en-US": "en", "de-jwyiGF9oSqQ": "de", "zh-Hans": "zh", "fr": "fr", "": ""} {
		if got := BaseLang(in); got != want {
			t.Errorf("BaseLang(%q) = %q, want %q", in, got, want)
		}
	}
	if HumanSize(1362269481) != "1.3GiB" || HumanSize(500) != "500B" {
		t.Error("HumanSize")
	}
}
