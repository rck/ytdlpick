// Package selection describes what a user picked for a video in a way
// that can be carried over to other videos: selections are stored as
// attributes (resolution, codec, language, ...) and resolved against each
// video's actual format list, never as site specific format IDs.
package selection

import (
	"fmt"
	"math"
	"sort"
	"strings"

	"github.com/rck/ytdlpick/internal/ytdlp"
)

// VideoSel describes the wanted video stream.
type VideoSel struct {
	Best bool // yt-dlp's best video
	// Specific stream attributes, used when Best is false.
	Height int
	FPS    int
	Codec  string
	Ext    string
	Muxed  bool // stream carries audio as well
	HDR    bool
}

// AudioTrack describes one wanted audio stream.
type AudioTrack struct {
	Best bool   // best audio of language Lang
	Lang string // language tag
	// Specific stream attributes, used when Best is false.
	Codec string
	Ext   string
	ABR   float64
	DRC   bool
}

// AudioSel describes the wanted audio streams. Several tracks are merged
// into one file by yt-dlp. No tracks means yt-dlp's best audio.
type AudioSel struct {
	Tracks []AudioTrack
}

// BestAny reports whether plain best audio is wanted.
func (a AudioSel) BestAny() bool { return len(a.Tracks) == 0 }

// SubMode says what to do with selected subtitles.
type SubMode int

const (
	SubNone SubMode = iota
	SubSeparate
	SubEmbed
)

func (m SubMode) String() string {
	switch m {
	case SubSeparate:
		return "separate files"
	case SubEmbed:
		return "embedded"
	}
	return "none"
}

// SubLang is one wanted subtitle language.
type SubLang struct {
	Lang string
	Auto bool
}

// SubSel describes the wanted subtitles.
type SubSel struct {
	Mode  SubMode
	Langs []SubLang
}

// Active reports whether any subtitles will be downloaded.
func (s SubSel) Active() bool { return s.Mode != SubNone && len(s.Langs) > 0 }

// Selection is the complete choice for one video.
type Selection struct {
	Video VideoSel
	Audio AudioSel
	Subs  SubSel
}

// Default is best video, best audio, no subtitles.
func Default() Selection {
	return Selection{Video: VideoSel{Best: true}}
}

// FromVideoFormat builds a VideoSel from a concrete format.
func FromVideoFormat(f ytdlp.Format) VideoSel {
	return VideoSel{Height: f.Height, FPS: f.FPSInt(), Codec: f.VideoCodec(), Ext: f.Ext, Muxed: f.Kind() == ytdlp.KindMuxed, HDR: f.HDR()}
}

// FromAudioFormat builds an AudioTrack from a concrete format.
func FromAudioFormat(f ytdlp.Format) AudioTrack {
	return AudioTrack{Lang: f.Language, Codec: f.AudioCodec(), Ext: f.Ext, ABR: f.ABR, DRC: f.DRC()}
}

// BestAudioIn selects the best audio track of a language.
func BestAudioIn(lang string) AudioTrack { return AudioTrack{Best: true, Lang: lang} }

// Unmet lists the parts of a selection that a video cannot satisfy.
type Unmet struct {
	Video, Audio, Subs bool
}

// Any reports whether something is unmet.
func (u Unmet) Any() bool { return u.Video || u.Audio || u.Subs }

func (u Unmet) String() string {
	var parts []string
	if u.Video {
		parts = append(parts, "video")
	}
	if u.Audio {
		parts = append(parts, "audio")
	}
	if u.Subs {
		parts = append(parts, "subtitles")
	}
	return strings.Join(parts, ", ")
}

// ResolvedAudio is one audio track found in a concrete video.
type ResolvedAudio struct {
	Format *ytdlp.Format // nil means best audio of Lang
	Lang   string        // exact language tag present in this video
}

// Resolved is a selection applied to one concrete video.
type Resolved struct {
	Video    *ytdlp.Format   // nil means "best"
	Audio    []ResolvedAudio // empty means "best audio"
	Included bool            // audio comes with the muxed video stream
	Mode     SubMode
	Subs     []ytdlp.SubTrack
}

// Resolve applies the selection to a video. Parts that cannot be
// satisfied fall back to their defaults and are reported in Unmet.
func (s Selection) Resolve(info *ytdlp.Info) (Resolved, Unmet) {
	var r Resolved
	var u Unmet

	if !s.Video.Best {
		if f := MatchVideo(info, s.Video); f != nil {
			r.Video = f
		} else {
			u.Video = true
		}
	}
	if r.Video != nil && r.Video.Kind() == ytdlp.KindMuxed {
		r.Included = true
	} else if info.MuxedOnly() {
		r.Included = true
	} else {
		for _, t := range s.Audio.Tracks {
			switch {
			case t.Best:
				if lang, ok := matchLang(info.AudioLanguages(), t.Lang); ok {
					r.Audio = append(r.Audio, ResolvedAudio{Lang: lang})
				} else {
					u.Audio = true
				}
			default:
				if f := MatchAudio(info, t); f != nil {
					r.Audio = append(r.Audio, ResolvedAudio{Format: f, Lang: f.Language})
				} else {
					u.Audio = true
				}
			}
		}
	}

	r.Mode = s.Subs.Mode
	if s.Subs.Active() {
		for _, want := range s.Subs.Langs {
			if t := MatchSub(info, want); t != nil {
				r.Subs = append(r.Subs, *t)
			} else {
				u.Subs = true
			}
		}
		if len(r.Subs) == 0 {
			r.Mode = SubNone
		}
	} else {
		r.Mode = SubNone
	}
	return r, u
}

// Applies reports whether the selection can be used unchanged for a video.
func (s Selection) Applies(info *ytdlp.Info) bool {
	_, u := s.Resolve(info)
	return !u.Any()
}

// MatchVideo finds the format of a video that best matches the wanted
// attributes, or nil when none matches.
func MatchVideo(info *ytdlp.Info, want VideoSel) *ytdlp.Format {
	var best *ytdlp.Format
	for i := range info.Video {
		f := &info.Video[i]
		if f.Height != want.Height || f.FPSInt() != want.FPS || f.VideoCodec() != want.Codec ||
			f.Ext != want.Ext || (f.Kind() == ytdlp.KindMuxed) != want.Muxed || f.HDR() != want.HDR {
			continue
		}
		if best == nil || f.TBR > best.TBR {
			best = f
		}
	}
	return best
}

// MatchAudio finds the audio format that best matches the wanted
// attributes: same language (exact, then base language), codec and
// container, closest bitrate.
func MatchAudio(info *ytdlp.Info, want AudioTrack) *ytdlp.Format {
	lang, ok := matchLang(info.AudioLanguages(), want.Lang)
	if !ok && want.Lang != "" {
		return nil
	}
	var best *ytdlp.Format
	for i := range info.Audio {
		f := &info.Audio[i]
		if f.Language != lang || f.AudioCodec() != want.Codec || f.Ext != want.Ext || f.DRC() != want.DRC {
			continue
		}
		if best == nil || math.Abs(f.ABR-want.ABR) < math.Abs(best.ABR-want.ABR) {
			best = f
		}
	}
	return best
}

// MatchSub finds the subtitle track for a wanted language.
func MatchSub(info *ytdlp.Info, want SubLang) *ytdlp.SubTrack {
	var langs []string
	for _, t := range info.Subs {
		if t.Auto == want.Auto {
			langs = append(langs, t.Lang)
		}
	}
	lang, ok := matchLang(langs, want.Lang)
	if !ok {
		return nil
	}
	for i := range info.Subs {
		if info.Subs[i].Auto == want.Auto && info.Subs[i].Lang == lang {
			return &info.Subs[i]
		}
	}
	return nil
}

// matchLang picks the best candidate for a wanted language tag: an exact
// match first, then the same base language. An empty wanted tag matches
// the empty tag only.
func matchLang(candidates []string, want string) (string, bool) {
	for _, c := range candidates {
		if strings.EqualFold(c, want) {
			return c, true
		}
	}
	if want == "" {
		return "", false
	}
	base := ytdlp.BaseLang(want)
	var same []string
	for _, c := range candidates {
		if ytdlp.BaseLang(c) == base {
			same = append(same, c)
		}
	}
	if len(same) == 0 {
		return "", false
	}
	sort.Strings(same)
	return same[0], true
}

// FormatSelector returns the value for yt-dlp's -f option.
func (r Resolved) FormatSelector() string {
	video := "bv*"
	if r.Video != nil {
		if r.Included {
			return r.Video.ID
		}
		video = r.Video.ID
	}
	if r.Included {
		return "bv*+ba/b"
	}
	if r.Video == nil && len(r.Audio) == 0 {
		return "bv*+ba/b"
	}
	parts := []string{video}
	if len(r.Audio) == 0 {
		parts = append(parts, "ba")
	}
	for _, a := range r.Audio {
		if a.Format != nil {
			parts = append(parts, a.Format.ID)
		} else {
			parts = append(parts, "ba[language="+a.Lang+"]")
		}
	}
	return strings.Join(parts, "+")
}

// Args returns the yt-dlp download arguments for this resolution.
// Note: when both an uploaded and an auto generated track exist for the
// same language, yt-dlp prefers the uploaded one even if "auto" was
// chosen here.
func (r Resolved) Args() []string {
	args := []string{"-f", r.FormatSelector()}
	if len(r.Audio) > 1 && !r.Included {
		args = append(args, "--audio-multistreams")
	}
	if r.Mode == SubNone || len(r.Subs) == 0 {
		return args
	}
	var langs []string
	manual, auto := false, false
	for _, t := range r.Subs {
		langs = append(langs, t.Lang)
		if t.Auto {
			auto = true
		} else {
			manual = true
		}
	}
	if manual {
		args = append(args, "--write-subs")
	}
	if auto {
		args = append(args, "--write-auto-subs")
	}
	args = append(args, "--sub-langs", strings.Join(langs, ","))
	if r.Mode == SubEmbed {
		args = append(args, "--embed-subs")
	}
	return args
}

// VideoText describes the resolved video choice.
func (r Resolved) VideoText() string {
	if r.Video == nil {
		return "best video"
	}
	return ytdlp.VideoLabel(*r.Video)
}

// AudioText describes the resolved audio choice.
func (r Resolved) AudioText() string {
	if r.Included {
		return "included in video"
	}
	if len(r.Audio) == 0 {
		return "best audio"
	}
	var parts []string
	for _, a := range r.Audio {
		if a.Format != nil {
			parts = append(parts, ytdlp.AudioLabel(*a.Format))
		} else {
			parts = append(parts, fmt.Sprintf("best audio [%s]", a.Lang))
		}
	}
	return strings.Join(parts, " + ")
}

// SubsText describes the resolved subtitle choice.
func (r Resolved) SubsText() string {
	if r.Mode == SubNone || len(r.Subs) == 0 {
		return "none"
	}
	var langs []string
	for _, t := range r.Subs {
		l := t.Lang
		if t.Auto {
			l += "(auto)"
		}
		langs = append(langs, l)
	}
	return r.Mode.String() + ": " + strings.Join(langs, ",")
}
