package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/rck/ytdlpick/internal/selection"
	"github.com/rck/ytdlpick/internal/ytdlp"
)

// action is what a button in the selection view does.
type action int

const (
	actNext     action = iota // use the choice and ask for the next item
	actContinue               // keep applying the template to the rest
	actRest                   // use the choice for all remaining items
	actStart                  // last item: start downloading
	actAbort
)

type button struct {
	label string
	act   action
	key   string // hotkey
}

const (
	focusVideo = iota
	focusAudio
	focusSubs
	focusButtons
)

const (
	subItemNone = iota
	subItemSeparate
	subItemEmbed
	subItemHeader
	subItemFirstLang
)

// selectView lets the user pick video, audio and subtitles for one item.
type selectView struct {
	item  *Item
	info  *ytdlp.Info
	index int // position in the list, 1 based
	total int

	video, audio, subs pane
	audioLangs         []string // languages with a "best audio [lang]" entry
	audioFmtStart      int      // index of the first concrete audio format item
	focus              int
	expanded           int // pane kept open in the narrow layout
	buttons            []button
	button             int
	unmet              selection.Unmet
	note               string
	width, height      int
}

func newSelectView(it *Item, index, total int, preset selection.Selection, buttons []button, note string) *selectView {
	info := it.Info
	v := &selectView{item: it, info: info, index: index, total: total, buttons: buttons, note: note}

	vitems := []paneItem{{label: "best video"}}
	for _, f := range info.Video {
		vitems = append(vitems, paneItem{label: ytdlp.VideoLabel(f)})
	}
	v.video = newPane("Video", vitems)

	// Audio entries are checkboxes: several tracks can be merged into one
	// file. "best audio" is exclusive with everything else.
	aitems := []paneItem{{label: "best audio", check: true}}
	v.audioLangs = info.AudioLanguages()
	for _, l := range v.audioLangs {
		aitems = append(aitems, paneItem{label: fmt.Sprintf("best audio [%s] %s", l, ytdlp.LanguageName(l)), check: true})
	}
	if len(info.Audio) > 0 {
		aitems = append(aitems, paneItem{label: "── all tracks ──", header: true})
	}
	v.audioFmtStart = len(aitems)
	for _, f := range info.Audio {
		aitems = append(aitems, paneItem{label: ytdlp.AudioLabel(f), check: true})
	}
	v.audio = newPane("Audio", aitems)

	sitems := []paneItem{{label: "none"}, {label: "separate files"}, {label: "embed into video"}}
	if len(info.Subs) > 0 {
		sitems = append(sitems, paneItem{label: "── languages ──", header: true})
	} else {
		sitems = append(sitems, paneItem{label: "── no subtitles available ──", header: true})
	}
	for _, t := range info.Subs {
		sitems = append(sitems, paneItem{label: t.Label(), check: true})
	}
	v.subs = newPane("Subtitles", sitems)

	v.applySelection(preset)
	v.setFocus(focusVideo)
	return v
}

// applySelection preselects the panes from a selection, falling back to
// defaults for parts the item cannot satisfy.
func (v *selectView) applySelection(sel selection.Selection) {
	r, u := sel.Resolve(v.info)
	v.unmet = u

	v.video.radio = 0
	if r.Video != nil {
		for i := range v.info.Video {
			if v.info.Video[i].ID == r.Video.ID {
				v.video.radio = i + 1
			}
		}
	}
	v.video.moveTo(v.video.radio)

	v.audio.checks = map[int]bool{}
	first := -1
	for _, a := range r.Audio {
		idx := -1
		if a.Format != nil {
			for i := range v.info.Audio {
				if v.info.Audio[i].ID == a.Format.ID {
					idx = v.audioFmtStart + i
				}
			}
		} else {
			for i, l := range v.audioLangs {
				if l == a.Lang {
					idx = i + 1
				}
			}
		}
		if idx >= 0 {
			v.audio.checks[idx] = true
			if first < 0 {
				first = idx
			}
		}
	}
	if first < 0 {
		v.audio.checks[0] = true
		first = 0
	}
	v.audio.moveTo(first)
	v.syncAudio()

	v.subs.radio = subItemNone
	switch r.Mode {
	case selection.SubSeparate:
		v.subs.radio = subItemSeparate
	case selection.SubEmbed:
		v.subs.radio = subItemEmbed
	}
	v.subs.checks = map[int]bool{}
	for _, t := range r.Subs {
		for i := range v.info.Subs {
			if v.info.Subs[i].Lang == t.Lang && v.info.Subs[i].Auto == t.Auto {
				v.subs.checks[subItemFirstLang+i] = true
			}
		}
	}
}

// syncAudio disables the audio pane when the chosen video carries audio.
func (v *selectView) syncAudio() {
	switch {
	case v.info.MuxedOnly():
		v.audio.setDisabled("This site only offers combined streams, the audio is part of the video stream.")
	case v.video.radio > 0 && v.info.Video[v.video.radio-1].Kind() == ytdlp.KindMuxed:
		v.audio.setDisabled("The selected video stream already contains audio.")
	default:
		v.audio.setDisabled("")
	}
}

// selection reads the user's choice out of the panes.
func (v *selectView) selection() selection.Selection {
	var s selection.Selection
	if v.video.radio <= 0 {
		s.Video = selection.VideoSel{Best: true}
	} else {
		s.Video = selection.FromVideoFormat(v.info.Video[v.video.radio-1])
	}
	if v.audio.disabled == "" && !v.audio.checks[0] {
		for i := 1; i < len(v.audio.items); i++ {
			if !v.audio.checks[i] {
				continue
			}
			if i < v.audioFmtStart {
				s.Audio.Tracks = append(s.Audio.Tracks, selection.BestAudioIn(v.audioLangs[i-1]))
			} else {
				s.Audio.Tracks = append(s.Audio.Tracks, selection.FromAudioFormat(v.info.Audio[i-v.audioFmtStart]))
			}
		}
	}
	switch v.subs.radio {
	case subItemSeparate:
		s.Subs.Mode = selection.SubSeparate
	case subItemEmbed:
		s.Subs.Mode = selection.SubEmbed
	}
	for i, t := range v.info.Subs {
		if v.subs.checks[subItemFirstLang+i] {
			s.Subs.Langs = append(s.Subs.Langs, selection.SubLang{Lang: t.Lang, Auto: t.Auto})
		}
	}
	return s
}

func (v *selectView) setFocus(f int) {
	v.focus = f
	v.video.focused = f == focusVideo
	v.audio.focused = f == focusAudio
	v.subs.focused = f == focusSubs
	if f != focusButtons {
		v.expanded = f
	}
}

func (v *selectView) focusedPane() *pane {
	switch v.focus {
	case focusVideo:
		return &v.video
	case focusAudio:
		return &v.audio
	case focusSubs:
		return &v.subs
	}
	return nil
}

func (v *selectView) cycleFocus(delta int) {
	f := v.focus
	for range 4 {
		f = (f + delta + 4) % 4
		if f == focusAudio && v.audio.disabled != "" {
			continue
		}
		break
	}
	v.setFocus(f)
}

// update handles a key; it returns an action when a button was activated.
func (v *selectView) update(k tea.KeyMsg) (action, bool) {
	p := v.focusedPane()
	filtering := p != nil && p.filtering

	if !filtering {
		switch k.String() {
		case "tab":
			v.cycleFocus(1)
			return 0, false
		case "shift+tab":
			v.cycleFocus(-1)
			return 0, false
		case "q":
			return actAbort, true
		}
		for i, b := range v.buttons {
			if b.key != "" && k.String() == b.key {
				v.button = i
				return b.act, true
			}
		}
	}

	if v.focus == focusButtons {
		switch k.String() {
		case "left", "h":
			v.button = max(0, v.button-1)
		case "right", "l":
			v.button = min(len(v.buttons)-1, v.button+1)
		case "enter", " ":
			return v.buttons[v.button].act, true
		case "up", "k":
			v.setFocus(v.expanded)
		}
		return 0, false
	}

	if p.handleKey(k) {
		return 0, false
	}
	switch k.String() {
	case "enter", " ":
		if idx, ok := p.toggle(); ok {
			v.afterToggle(idx)
		}
	case "left", "h":
		v.cycleFocus(-1)
	case "right", "l":
		v.cycleFocus(1)
	}
	return 0, false
}

// afterToggle keeps the panes consistent after a selection change.
func (v *selectView) afterToggle(idx int) {
	switch v.focus {
	case focusVideo:
		v.syncAudio()
	case focusAudio:
		// "best audio" excludes specific tracks and vice versa; never
		// leave nothing selected.
		if idx == 0 && v.audio.checks[0] {
			v.audio.checks = map[int]bool{0: true}
		} else if idx != 0 && v.audio.checks[idx] {
			delete(v.audio.checks, 0)
		}
		any := false
		for _, on := range v.audio.checks {
			any = any || on
		}
		if !any {
			v.audio.checks[0] = true
		}
	case focusSubs:
		if idx >= subItemFirstLang && v.subs.radio == subItemNone && v.subs.checks[idx] {
			v.subs.radio = subItemSeparate
		}
	}
}

func (v *selectView) setSize(w, h int) { v.width, v.height = w, h }

func (v *selectView) header() string {
	var b strings.Builder
	title := fmt.Sprintf("%d/%d  %s", v.index, v.total, v.info.Title)
	b.WriteString(ansi.Truncate(styTitle.Render(title), v.width, "…"))
	b.WriteString("\n")
	var meta []string
	if v.info.Channel != "" {
		meta = append(meta, v.info.Channel)
	} else if v.info.Uploader != "" {
		meta = append(meta, v.info.Uploader)
	}
	if v.info.DurationString != "" {
		meta = append(meta, v.info.DurationString)
	}
	if v.info.Extractor != "" {
		meta = append(meta, v.info.Extractor)
	}
	meta = append(meta, v.item.URL)
	b.WriteString(ansi.Truncate(styMuted.Render(strings.Join(meta, " · ")), v.width, "…"))
	if v.note != "" {
		b.WriteString("\n")
		b.WriteString(ansi.Truncate(styWarn.Render(v.note), v.width, "…"))
	}
	return b.String()
}

func (v *selectView) buttonsView() string {
	var parts []string
	for i, b := range v.buttons {
		sty := styButton
		if v.focus == focusButtons && i == v.button {
			sty = styButtonFocused
			if b.act == actAbort {
				sty = styButtonDanger
			}
		}
		label := b.label
		if b.key != "" {
			label += styMuted.Render(" (" + b.key + ")")
		}
		parts = append(parts, sty.Render(label))
	}
	return lipgloss.JoinHorizontal(lipgloss.Top, parts...)
}

func (v *selectView) help() string {
	return styHelp.Render("tab/←→ switch pane · ↑↓ move · space/enter select · / filter · q abort")
}

func (v *selectView) view() string {
	head := v.header()
	buttons := v.buttonsView()
	help := v.help()
	used := lipgloss.Height(head) + lipgloss.Height(buttons) + lipgloss.Height(help) + 2
	avail := max(6, v.height-used)

	var panes string
	if v.width >= 110 {
		w := v.width / 3
		for _, p := range []*pane{&v.video, &v.audio, &v.subs} {
			p.width, p.height = w, avail
		}
		v.subs.width = v.width - 2*w
		panes = lipgloss.JoinHorizontal(lipgloss.Top, v.video.view(), v.audio.view(), v.subs.view())
	} else {
		var lines []string
		for i, p := range []*pane{&v.video, &v.audio, &v.subs} {
			if i == v.expanded {
				p.width, p.height = v.width, avail-2
				lines = append(lines, p.view())
			} else {
				lines = append(lines, " "+p.collapsed(v.width-1))
			}
		}
		panes = lipgloss.JoinVertical(lipgloss.Left, lines...)
	}
	return lipgloss.JoinVertical(lipgloss.Left, head, "", panes, buttons, help)
}
