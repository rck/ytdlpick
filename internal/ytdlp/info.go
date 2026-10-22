// Package ytdlp wraps the yt-dlp command line tool: it fetches media
// information as JSON and turns it into a cleaned, sorted model that the
// TUI and the selection logic can work with.
package ytdlp

import (
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"strings"
)

// Kind classifies a format by what streams it carries.
type Kind int

const (
	KindOther Kind = iota // storyboards, images, ...
	KindVideo             // video only
	KindAudio             // audio only
	KindMuxed             // video and audio in one stream
)

// Format is one entry of the "formats" array in yt-dlp's JSON output.
// JSON null values leave the zero value, so "" means unknown for strings.
type Format struct {
	ID             string  `json:"format_id"`
	Ext            string  `json:"ext"`
	VCodec         string  `json:"vcodec"`
	ACodec         string  `json:"acodec"`
	Height         int     `json:"height"`
	Width          int     `json:"width"`
	FPS            float64 `json:"fps"`
	TBR            float64 `json:"tbr"`
	VBR            float64 `json:"vbr"`
	ABR            float64 `json:"abr"`
	Language       string  `json:"language"`
	LangPref       int     `json:"language_preference"`
	Note           string  `json:"format_note"`
	Filesize       int64   `json:"filesize"`
	FilesizeApprox int64   `json:"filesize_approx"`
	HasDRM         bool    `json:"has_drm"`
	Protocol       string  `json:"protocol"`
	DynamicRange   string  `json:"dynamic_range"`
	AudioChannels  int     `json:"audio_channels"`
}

// Kind returns what streams the format carries.
func (f Format) Kind() Kind {
	v, a := f.VCodec == "none", f.ACodec == "none"
	switch {
	case v && a:
		return KindOther
	case v:
		return KindAudio
	case a:
		return KindVideo
	default:
		return KindMuxed
	}
}

// VideoCodec returns the normalized codec family of the video stream.
func (f Format) VideoCodec() string { return CodecFamily(f.VCodec) }

// AudioCodec returns the normalized codec family of the audio stream.
func (f Format) AudioCodec() string { return CodecFamily(f.ACodec) }

// FPSInt returns the frame rate rounded to an integer.
func (f Format) FPSInt() int { return int(math.Round(f.FPS)) }

// HDR reports whether the format is marked as high dynamic range.
func (f Format) HDR() bool {
	dr := strings.ToUpper(f.DynamicRange)
	return dr != "" && dr != "SDR"
}

// DRC reports whether the audio has dynamic range compression applied
// (YouTube offers such variants next to the plain ones).
func (f Format) DRC() bool {
	return strings.Contains(strings.ToUpper(f.Note), "DRC") || strings.HasSuffix(f.ID, "-drc")
}

// Original reports whether this audio track is the original language.
func (f Format) Original() bool {
	return f.LangPref >= 10 || strings.Contains(strings.ToLower(f.Note), "original")
}

// Size returns the best known size estimate and whether it is approximate.
func (f Format) Size() (int64, bool) {
	if f.Filesize > 0 {
		return f.Filesize, false
	}
	return f.FilesizeApprox, true
}

// subEntry is one entry of a subtitle language list in the JSON.
type subEntry struct {
	Ext  string `json:"ext"`
	Name string `json:"name"`
}

// SubTrack is one subtitle language, either uploaded or auto generated.
type SubTrack struct {
	Lang string // exact key as used by yt-dlp, e.g. "en", "de-jwyiGF9oSqQ"
	Name string // human readable, e.g. "German - Deutsch"
	Auto bool   // automatically generated captions
}

// Label is the display text of a subtitle track.
func (t SubTrack) Label() string {
	s := t.Lang
	if t.Name != "" && !strings.EqualFold(t.Name, t.Lang) {
		s += "  " + t.Name
	}
	if t.Auto {
		s += "  (auto)"
	}
	return s
}

// Info is the parsed yt-dlp JSON for one URL. A playlist carries Entries,
// a video carries Formats.
type Info struct {
	Type           string                `json:"_type"`
	ID             string                `json:"id"`
	Title          string                `json:"title"`
	WebpageURL     string                `json:"webpage_url"`
	OriginalURL    string                `json:"original_url"`
	URL            string                `json:"url"`
	Channel        string                `json:"channel"`
	Uploader       string                `json:"uploader"`
	Extractor      string                `json:"extractor_key"`
	Duration       float64               `json:"duration"`
	DurationString string                `json:"duration_string"`
	IsLive         bool                  `json:"is_live"`
	RawFormats     []Format              `json:"formats"`
	RawSubtitles   map[string][]subEntry `json:"subtitles"`
	RawAutoCaps    map[string][]subEntry `json:"automatic_captions"`
	Entries        []json.RawMessage     `json:"entries"`

	// Derived, filled by Prepare.
	Video []Format   // video-only and muxed formats, best first
	Audio []Format   // audio-only formats, original language and best first
	Subs  []SubTrack // uploaded subtitles first, then auto captions
}

// IsPlaylist reports whether the info describes a list of other entries.
func (i *Info) IsPlaylist() bool {
	return i.Type == "playlist" || i.Type == "multi_video"
}

// IsVideo reports whether the info describes a downloadable media item.
func (i *Info) IsVideo() bool {
	return !i.IsPlaylist() && len(i.RawFormats) > 0
}

// DownloadURL is the URL to hand to yt-dlp for downloading this item.
func (i *Info) DownloadURL() string {
	for _, u := range []string{i.WebpageURL, i.OriginalURL, i.URL} {
		if u != "" {
			return u
		}
	}
	return ""
}

// MuxedOnly reports whether the site offers no separate audio streams.
func (i *Info) MuxedOnly() bool {
	return len(i.Audio) == 0
}

// AudioLanguages returns the distinct audio languages, original first.
func (i *Info) AudioLanguages() []string {
	seen := map[string]bool{}
	var langs []string
	for _, f := range i.Audio {
		if f.Language == "" || seen[f.Language] {
			continue
		}
		seen[f.Language] = true
		langs = append(langs, f.Language)
	}
	return langs
}

// Parse decodes one yt-dlp JSON document and prepares the derived lists.
func Parse(data []byte) (*Info, error) {
	var info Info
	if err := json.Unmarshal(data, &info); err != nil {
		return nil, fmt.Errorf("decode yt-dlp JSON: %w", err)
	}
	info.Prepare()
	return &info, nil
}

// Prepare fills the derived lists from the raw JSON fields.
func (i *Info) Prepare() {
	i.Video, i.Audio = cleanFormats(i.RawFormats)
	i.Subs = subTracks(i.RawSubtitles, false)
	i.Subs = append(i.Subs, subTracks(i.RawAutoCaps, true)...)
}

// cleanFormats drops unusable formats, removes protocol duplicates and
// sorts the rest for display.
func cleanFormats(all []Format) (video, audio []Format) {
	type key struct {
		kind       Kind
		height     int
		fps        int
		codec, ext string
		lang       string
		abr        int
		drc, hdr   bool
	}
	best := map[key]Format{}
	var order []key
	for _, f := range all {
		k := f.Kind()
		if k == KindOther || f.HasDRM || f.Ext == "mhtml" {
			continue
		}
		if k == KindAudio && f.ACodec == "" && f.ABR == 0 && f.TBR == 0 {
			continue // HLS placeholder tracks without any usable description
		}
		fk := key{kind: k, height: f.Height, fps: f.FPSInt(), ext: f.Ext, lang: f.Language, drc: f.DRC(), hdr: f.HDR()}
		if k == KindAudio {
			fk.codec = f.AudioCodec()
			fk.abr = int(math.Round(f.ABR / 16))
		} else {
			fk.codec = f.VideoCodec()
		}
		prev, ok := best[fk]
		if !ok {
			best[fk] = f
			order = append(order, fk)
			continue
		}
		// Same stream over different protocols: prefer plain https, then
		// the better described one.
		if preferFormat(f, prev) {
			best[fk] = f
		}
	}
	for _, k := range order {
		f := best[k]
		if k.kind == KindAudio {
			audio = append(audio, f)
		} else {
			video = append(video, f)
		}
	}
	sort.SliceStable(video, func(a, b int) bool {
		x, y := video[a], video[b]
		if x.Height != y.Height {
			return x.Height > y.Height
		}
		if x.FPSInt() != y.FPSInt() {
			return x.FPSInt() > y.FPSInt()
		}
		if (x.Kind() == KindMuxed) != (y.Kind() == KindMuxed) {
			return x.Kind() != KindMuxed
		}
		if plainHTTP(x) != plainHTTP(y) {
			return plainHTTP(x)
		}
		return x.TBR > y.TBR
	})
	sort.SliceStable(audio, func(a, b int) bool {
		x, y := audio[a], audio[b]
		if x.Original() != y.Original() {
			return x.Original()
		}
		if x.Language != y.Language {
			return x.Language < y.Language
		}
		if x.DRC() != y.DRC() {
			return !x.DRC()
		}
		return x.ABR > y.ABR
	})
	return video, audio
}

func plainHTTP(f Format) bool { return f.Protocol == "https" || f.Protocol == "http" }

func preferFormat(a, b Format) bool {
	if pa, pb := plainHTTP(a), plainHTTP(b); pa != pb {
		return pa
	}
	sa, _ := a.Size()
	sb, _ := b.Size()
	if (sa > 0) != (sb > 0) {
		return sa > 0
	}
	return a.TBR > b.TBR
}

func subTracks(m map[string][]subEntry, auto bool) []SubTrack {
	var tracks []SubTrack
	for lang, entries := range m {
		t := SubTrack{Lang: lang, Auto: auto}
		for _, e := range entries {
			if e.Name != "" {
				t.Name = e.Name
				break
			}
		}
		if t.Name == "" {
			t.Name = LanguageName(lang)
		}
		tracks = append(tracks, t)
	}
	sort.Slice(tracks, func(a, b int) bool { return tracks[a].Lang < tracks[b].Lang })
	return tracks
}

// CodecFamily reduces a codec string like "avc1.64002A" to a short family
// name used for matching and display.
func CodecFamily(c string) string {
	c = strings.ToLower(strings.TrimSpace(c))
	if c == "" || c == "none" {
		return c
	}
	fam := strings.SplitN(c, ".", 2)[0]
	switch fam {
	case "vp09":
		return "vp9"
	case "avc3":
		return "avc1"
	case "hvc1":
		return "hev1"
	case "mp4a":
		return "aac"
	}
	return fam
}

// BaseLang reduces a language tag to its primary subtag: "en-US" -> "en",
// "de-jwyiGF9oSqQ" -> "de". Used for matching across different videos.
func BaseLang(lang string) string {
	lang = strings.ToLower(lang)
	if i := strings.IndexAny(lang, "-_"); i > 0 {
		return lang[:i]
	}
	return lang
}

// HumanSize formats a byte count.
func HumanSize(n int64) string {
	const unit = 1024
	if n < unit {
		return fmt.Sprintf("%dB", n)
	}
	div, exp := int64(unit), 0
	for m := n / unit; m >= unit; m /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f%ciB", float64(n)/float64(div), "KMGTPE"[exp])
}

// VideoLabel is the one line display text of a video format.
func VideoLabel(f Format) string {
	var parts []string
	res := ""
	if f.Height > 0 {
		res = fmt.Sprintf("%dp", f.Height)
		if fps := f.FPSInt(); fps > 0 && fps != 30 {
			res += fmt.Sprintf("%d", fps)
		}
	} else if f.Note != "" {
		res = f.Note
	} else {
		res = f.ID
	}
	parts = append(parts, res)
	if c := f.VideoCodec(); c != "" {
		parts = append(parts, c)
	}
	if f.Ext != "" {
		parts = append(parts, f.Ext)
	}
	if f.HDR() {
		parts = append(parts, "HDR")
	}
	if f.Kind() == KindMuxed {
		parts = append(parts, "+audio")
	}
	if s, approx := f.Size(); s > 0 {
		if approx {
			parts = append(parts, "~"+HumanSize(s))
		} else {
			parts = append(parts, HumanSize(s))
		}
	}
	if n := strings.ToLower(f.Note); strings.Contains(n, "premium") {
		parts = append(parts, "(premium)")
	}
	return strings.Join(parts, " ")
}

// AudioLabel is the one line display text of an audio format.
func AudioLabel(f Format) string {
	var parts []string
	if f.Language != "" {
		parts = append(parts, "["+f.Language+"]")
	}
	if c := f.AudioCodec(); c != "" {
		parts = append(parts, c)
	}
	if f.ABR > 0 {
		parts = append(parts, fmt.Sprintf("%dk", int(math.Round(f.ABR))))
	}
	if f.Ext != "" {
		parts = append(parts, f.Ext)
	}
	if f.AudioChannels > 2 {
		parts = append(parts, fmt.Sprintf("%dch", f.AudioChannels))
	}
	if f.DRC() {
		parts = append(parts, "DRC")
	}
	if f.Original() {
		parts = append(parts, "original")
	}
	if s, approx := f.Size(); s > 0 {
		if approx {
			parts = append(parts, "~"+HumanSize(s))
		} else {
			parts = append(parts, HumanSize(s))
		}
	}
	if len(parts) == 0 {
		return f.ID
	}
	return strings.Join(parts, " ")
}

// LanguageName returns a display name for common language tags.
func LanguageName(tag string) string {
	if n, ok := langNames[strings.ToLower(tag)]; ok {
		return n
	}
	if n, ok := langNames[BaseLang(tag)]; ok {
		return n
	}
	return tag
}

var langNames = map[string]string{
	"ar": "Arabic", "bn": "Bangla", "cs": "Czech", "da": "Danish", "de": "German",
	"el": "Greek", "en": "English", "en-us": "English (US)", "en-gb": "English (UK)",
	"es": "Spanish", "fa": "Persian", "fi": "Finnish", "fr": "French", "he": "Hebrew",
	"hi": "Hindi", "hr": "Croatian", "hu": "Hungarian", "id": "Indonesian",
	"it": "Italian", "iw": "Hebrew", "ja": "Japanese", "ko": "Korean", "ml": "Malayalam",
	"mr": "Marathi", "nl": "Dutch", "no": "Norwegian", "pa": "Punjabi", "pl": "Polish",
	"pt": "Portuguese", "ro": "Romanian", "ru": "Russian", "sk": "Slovak", "sl": "Slovenian",
	"sq": "Albanian", "sv": "Swedish", "ta": "Tamil", "te": "Telugu", "th": "Thai",
	"tr": "Turkish", "uk": "Ukrainian", "vi": "Vietnamese", "zh": "Chinese",
	"zh-hans": "Chinese (Simplified)", "zh-hant": "Chinese (Traditional)",
}
