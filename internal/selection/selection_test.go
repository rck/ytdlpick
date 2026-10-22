package selection

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rck/ytdlpick/internal/ytdlp"
)

func load(t *testing.T, name string) *ytdlp.Info {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("..", "ytdlp", "testdata", name))
	if err != nil {
		t.Fatal(err)
	}
	info, err := ytdlp.Parse(data)
	if err != nil {
		t.Fatal(err)
	}
	return info
}

func findVideo(t *testing.T, info *ytdlp.Info, id string) ytdlp.Format {
	t.Helper()
	for _, f := range info.Video {
		if f.ID == id {
			return f
		}
	}
	t.Fatalf("video format %s not found", id)
	return ytdlp.Format{}
}

func TestDefault(t *testing.T) {
	bbb := load(t, "video_bbb.json")
	r, u := Default().Resolve(bbb)
	if u.Any() {
		t.Fatalf("default unmet: %v", u)
	}
	if got := strings.Join(r.Args(), " "); got != "-f bv*+ba/b" {
		t.Errorf("args = %q", got)
	}
	if r.VideoText() != "best video" || r.AudioText() != "best audio" || r.SubsText() != "none" {
		t.Errorf("texts: %q %q %q", r.VideoText(), r.AudioText(), r.SubsText())
	}
}

func TestAudioLanguageCarryOver(t *testing.T) {
	bbb := load(t, "video_bbb.json")
	multi := load(t, "video_multiaudio.json")

	sel := Default()
	sel.Audio.Tracks = []AudioTrack{BestAudioIn("de")}
	r, u := sel.Resolve(multi)
	if u.Any() || len(r.Audio) != 1 || r.Audio[0].Lang != "de" {
		t.Fatalf("de on multi: unmet=%v audio=%+v", u, r.Audio)
	}
	if got := r.FormatSelector(); got != "bv*+ba[language=de]" {
		t.Errorf("selector = %q", got)
	}
	if _, u := sel.Resolve(bbb); !u.Audio || u.Video || u.Subs {
		t.Errorf("de on bbb should only miss audio, got %v", u)
	}

	// Base language match: "en" wanted, only "en-US" available.
	sel.Audio.Tracks = []AudioTrack{BestAudioIn("en")}
	r, u = sel.Resolve(multi)
	if u.Any() || len(r.Audio) != 1 || r.Audio[0].Lang != "en-US" {
		t.Errorf("en on multi: unmet=%v audio=%+v", u, r.Audio)
	}
	if r.AudioText() != "best audio [en-US]" {
		t.Errorf("text = %q", r.AudioText())
	}
}

func TestMultipleAudioTracks(t *testing.T) {
	bbb := load(t, "video_bbb.json")
	multi := load(t, "video_multiaudio.json")
	sel := Default()
	sel.Audio.Tracks = []AudioTrack{BestAudioIn("en"), BestAudioIn("de")}
	r, u := sel.Resolve(multi)
	if u.Any() {
		t.Fatalf("unmet %v", u)
	}
	args := strings.Join(r.Args(), " ")
	if args != "-f bv*+ba[language=en-US]+ba[language=de] --audio-multistreams" {
		t.Errorf("args = %q", args)
	}
	if r.AudioText() != "best audio [en-US] + best audio [de]" {
		t.Errorf("text = %q", r.AudioText())
	}
	// bbb has no language tags: both unmet, nothing resolved.
	r, u = sel.Resolve(bbb)
	if !u.Audio || len(r.Audio) != 0 || strings.Contains(strings.Join(r.Args(), " "), "multistreams") {
		t.Errorf("bbb: unmet=%v audio=%+v args=%v", u, r.Audio, r.Args())
	}
}

func TestSpecificFormats(t *testing.T) {
	bbb := load(t, "video_bbb.json")
	multi := load(t, "video_multiaudio.json")

	v := findVideo(t, bbb, "303") // 1080p60 vp9 webm
	sel := Default()
	sel.Video = FromVideoFormat(v)
	r, u := sel.Resolve(bbb)
	if u.Any() || r.Video == nil || r.Video.ID != "303" {
		t.Fatalf("own format should resolve to itself: unmet=%v video=%v", u, r.Video)
	}
	if got := r.FormatSelector(); got != "303+ba" {
		t.Errorf("selector = %q", got)
	}
	// The other video is 30 fps only, so a 60 fps choice does not fit.
	if _, u := sel.Resolve(multi); !u.Video {
		t.Error("1080p60 should not apply to a 30fps video")
	}

	// Specific audio: German opus 128k on multi, carried to bbb has no
	// language at all -> unmet. Carried to itself -> same ID.
	var de ytdlp.Format
	for _, f := range multi.Audio {
		if f.Language == "de" && f.AudioCodec() == "opus" && !f.DRC() && f.ABR > 100 {
			de = f
		}
	}
	if de.ID == "" {
		t.Fatal("no german opus track in fixture")
	}
	sel = Default()
	sel.Audio.Tracks = []AudioTrack{FromAudioFormat(de)}
	r, u = sel.Resolve(multi)
	if u.Any() || len(r.Audio) != 1 || r.Audio[0].Format == nil || r.Audio[0].Format.ID != de.ID {
		t.Errorf("audio self resolve: unmet=%v audio=%+v", u, r.Audio)
	}
	if got := r.FormatSelector(); got != "bv*+"+de.ID {
		t.Errorf("selector = %q", got)
	}
	if _, u := sel.Resolve(bbb); !u.Audio {
		t.Error("german track should not apply to bbb")
	}
}

func TestSubtitles(t *testing.T) {
	sintel := load(t, "video_sintel.json")
	multi := load(t, "video_multiaudio.json")

	sel := Default()
	sel.Subs = SubSel{Mode: SubEmbed, Langs: []SubLang{{Lang: "de"}, {Lang: "en", Auto: true}}}
	r, u := sel.Resolve(sintel)
	if u.Any() {
		t.Fatalf("unmet %v", u)
	}
	args := strings.Join(r.Args(), " ")
	// Sintel's German subtitles use a suffixed key, matched by base language.
	if !strings.Contains(args, "--write-subs") || !strings.Contains(args, "--write-auto-subs") ||
		!strings.Contains(args, "--sub-langs de-jwyiGF9oSqQ,en") || !strings.Contains(args, "--embed-subs") {
		t.Errorf("args = %q", args)
	}
	if r.SubsText() != "embedded: de-jwyiGF9oSqQ,en(auto)" {
		t.Errorf("text = %q", r.SubsText())
	}
	r, u = sel.Resolve(multi)
	if u.Any() || len(r.Subs) != 2 || r.Subs[0].Lang != "de" {
		t.Errorf("multi: unmet=%v subs=%v", u, r.Subs)
	}

	// A language nobody has.
	sel.Subs.Langs = []SubLang{{Lang: "xx"}}
	r, u = sel.Resolve(multi)
	if !u.Subs || r.Mode != SubNone || len(r.Args()) != 2 {
		t.Errorf("missing language: unmet=%v mode=%v args=%v", u, r.Mode, r.Args())
	}

	// Mode none ignores languages.
	sel.Subs = SubSel{Mode: SubNone, Langs: []SubLang{{Lang: "de"}}}
	if _, u := sel.Resolve(multi); u.Any() {
		t.Error("mode none must always apply")
	}
}

func TestMuxedOnly(t *testing.T) {
	info := &ytdlp.Info{Type: "video", Title: "x", RawFormats: []ytdlp.Format{
		{ID: "hd", Ext: "mp4", VCodec: "avc1", ACodec: "aac", Height: 720, TBR: 1000},
		{ID: "sd", Ext: "mp4", VCodec: "avc1", ACodec: "aac", Height: 360, TBR: 500},
	}}
	info.Prepare()
	if !info.MuxedOnly() {
		t.Fatal("expected muxed only")
	}
	r, u := Default().Resolve(info)
	if u.Any() || !r.Included || r.FormatSelector() != "bv*+ba/b" || r.AudioText() != "included in video" {
		t.Errorf("default on muxed: %v %+v %q", u, r, r.FormatSelector())
	}
	sel := Default()
	sel.Video = FromVideoFormat(info.Video[1])
	sel.Audio.Tracks = []AudioTrack{BestAudioIn("de")} // irrelevant, audio is included
	r, u = sel.Resolve(info)
	if u.Any() || r.FormatSelector() != "sd" {
		t.Errorf("muxed specific: unmet=%v selector=%q", u, r.FormatSelector())
	}
}
