package tui

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/rck/ytdlpick/internal/selection"
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

func key(s string) tea.KeyMsg {
	switch s {
	case "enter":
		return tea.KeyMsg{Type: tea.KeyEnter}
	case "tab":
		return tea.KeyMsg{Type: tea.KeyTab}
	case " ":
		return tea.KeyMsg{Type: tea.KeySpace}
	}
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)}
}

func testApp(urls ...string) *App {
	a := newApp(context.Background(), ytdlp.Runner{Bin: "false"}, urls, 1)
	a.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	return a
}

func (a *App) feed(t *testing.T, i int, info *ytdlp.Info) {
	t.Helper()
	a.Update(fetchedMsg{item: a.items[i], info: info})
}

func (a *App) press(keys ...string) {
	for _, k := range keys {
		a.Update(key(k))
	}
}

// chooseAudioLang toggles "best audio [lang]" in the current view via keys.
func (a *App) chooseAudioLang(t *testing.T, lang string) {
	t.Helper()
	for i, l := range a.sel.audioLangs {
		if l == lang {
			a.sel.setFocus(focusAudio)
			a.sel.audio.moveTo(i + 1)
			a.press(" ")
			return
		}
	}
	t.Fatalf("language %s not offered", lang)
}

func TestSetForTheRestWithException(t *testing.T) {
	multi := load(t, "video_multiaudio.json")
	bbb := load(t, "video_bbb.json")
	a := testApp("u0", "u1", "u2", "u3")
	a.feed(t, 0, multi)
	a.feed(t, 1, multi)
	a.feed(t, 2, bbb)
	a.feed(t, 3, multi)
	if a.screen != screenSelect || a.current != 0 {
		t.Fatalf("screen=%v current=%d", a.screen, a.current)
	}
	a.chooseAudioLang(t, "de")
	a.press("r") // set for the rest
	// Item 1 has German -> auto. Item 2 has no languages -> exception.
	if a.current != 2 || a.screen != screenSelect {
		t.Fatalf("expected exception on item 2, got current=%d screen=%v", a.current, a.screen)
	}
	if !a.items[1].Auto || len(a.items[1].Res.Audio) != 1 || a.items[1].Res.Audio[0].Lang != "de" {
		t.Errorf("item 1 not auto decided with de: %+v", a.items[1].Res)
	}
	if !strings.Contains(a.sel.note, "audio") {
		t.Errorf("note = %q", a.sel.note)
	}
	if a.sel.buttons[0].act != actContinue {
		t.Errorf("first button should be Continue, got %v", a.sel.buttons[0].label)
	}
	a.press("n") // continue with the template
	// Item 3 has German again -> auto, then the summary.
	if a.screen != screenSummary {
		t.Fatalf("expected summary, got %v (current %d)", a.screen, a.current)
	}
	if !a.items[3].Auto || len(a.items[3].Res.Audio) != 1 || a.items[3].Res.Audio[0].Lang != "de" {
		t.Errorf("item 3 should use the url 0 template: %+v", a.items[3].Res)
	}
	if a.items[2].Auto || len(a.items[2].Res.Audio) != 0 {
		t.Errorf("item 2 should be the manual fallback: %+v", a.items[2].Res)
	}
	if a.View() == "" {
		t.Error("empty summary view")
	}
	a.press("enter")
	if !a.finished || a.aborted {
		t.Errorf("finished=%v aborted=%v", a.finished, a.aborted)
	}
}

func TestNextCarriesPreviousChoice(t *testing.T) {
	multi := load(t, "video_multiaudio.json")
	a := testApp("u0", "u1")
	a.feed(t, 0, multi)
	a.feed(t, 1, multi)
	a.chooseAudioLang(t, "fr")
	a.chooseAudioLang(t, "de")
	if got := a.sel.selection().Audio.Tracks; len(got) != 2 || a.sel.audio.checks[0] {
		t.Fatalf("two tracks expected, got %+v (best=%v)", got, a.sel.audio.checks[0])
	}
	a.press("n")
	if a.current != 1 || a.screen != screenSelect {
		t.Fatalf("current=%d screen=%v", a.current, a.screen)
	}
	got := a.sel.selection().Audio.Tracks
	if len(got) != 2 || got[0].Lang != "de" || got[1].Lang != "fr" || !got[0].Best {
		t.Errorf("preselected audio = %+v", got)
	}
	// Re-checking "best audio" drops the specific tracks.
	a.sel.setFocus(focusAudio)
	a.sel.audio.moveTo(0)
	a.press(" ")
	if got := a.sel.selection().Audio; !got.BestAny() {
		t.Errorf("best audio should be exclusive, got %+v", got)
	}
	if a.sel.buttons[0].act != actStart {
		t.Errorf("last item should offer start, got %q", a.sel.buttons[0].label)
	}
	a.press("n")
	if !a.finished {
		t.Error("start should finish")
	}
}

func TestPlaylistExpansionAndErrors(t *testing.T) {
	pl := load(t, "playlist_flat.json")
	bbb := load(t, "video_bbb.json")
	a := testApp("before", "playlist", "after")
	a.feed(t, 1, pl)
	if len(a.items) != 6 {
		t.Fatalf("items after expansion = %d", len(a.items))
	}
	if a.items[1].Title == "" || a.items[1].Depth != 1 || a.items[5].URL != "after" {
		t.Errorf("expansion wrong: %+v / %+v", a.items[1], a.items[5])
	}
	if a.screen != screenLoading {
		t.Errorf("screen = %v", a.screen)
	}
	a.Update(fetchedMsg{item: a.items[0], err: errors.New("Video unavailable")})
	if a.screen != screenError {
		t.Fatalf("expected error screen, got %v", a.screen)
	}
	if !strings.Contains(a.View(), "Video unavailable") {
		t.Error("error text missing")
	}
	a.press("s") // skip
	if !a.items[0].Skipped || a.current != 1 || a.screen != screenLoading {
		t.Errorf("after skip: skipped=%v current=%d screen=%v", a.items[0].Skipped, a.current, a.screen)
	}
	for i := 1; i < 6; i++ {
		a.feed(t, i, bbb)
	}
	if a.screen != screenSelect || a.current != 1 {
		t.Fatalf("screen=%v current=%d", a.screen, a.current)
	}
	a.press("r") // defaults for the rest
	if a.screen != screenSummary {
		t.Fatalf("expected summary, got %v", a.screen)
	}
	n := 0
	for _, it := range a.items {
		if it.Decided() {
			n++
		}
	}
	if n != 5 {
		t.Errorf("decided = %d", n)
	}
	a.press("a")
	if !a.aborted {
		t.Error("abort from summary")
	}
}

func TestSelectViewKeys(t *testing.T) {
	sintel := load(t, "video_sintel.json")
	a := testApp("u0")
	a.feed(t, 0, sintel)
	v := a.sel
	// Move to the 2nd video entry and select it.
	a.press("j", " ")
	if v.video.radio != 1 {
		t.Errorf("video radio = %d", v.video.radio)
	}
	// Go to subtitles, filter for german, check it: mode flips to separate.
	a.press("tab", "tab")
	if v.focus != focusSubs {
		t.Fatalf("focus = %d", v.focus)
	}
	a.press("/", "g", "e", "r", "m", "enter", " ")
	s := v.selection()
	if s.Subs.Mode != selection.SubSeparate || len(s.Subs.Langs) != 1 || ytdlp.BaseLang(s.Subs.Langs[0].Lang) != "de" {
		t.Errorf("subs = %+v (visible %d)", s.Subs, len(v.subs.visible))
	}
	// Narrow layout renders too.
	a.Update(tea.WindowSizeMsg{Width: 70, Height: 20})
	if out := a.View(); !strings.Contains(out, "Subtitles") || !strings.Contains(out, "Video:") {
		t.Errorf("narrow view:\n%s", out)
	}
	// Buttons: tab to them, abort.
	a.press("tab", "l", "enter")
	if !a.aborted {
		t.Error("abort via button")
	}
}
