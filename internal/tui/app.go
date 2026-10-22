// Package tui implements the interactive part: fetching info for all
// URLs, asking the user what to download for each item and producing the
// download plan.
package tui

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/rck/ytdlpick/internal/selection"
	"github.com/rck/ytdlpick/internal/ytdlp"
)

const maxPlaylistDepth = 3

// Item is one URL to download, after playlist expansion.
type Item struct {
	URL     string
	Title   string // provisional title from a playlist listing
	Depth   int    // playlist nesting depth
	Info    *ytdlp.Info
	Err     error
	Skipped bool
	Sel     *selection.Selection // decided choice, nil while undecided
	Res     selection.Resolved
	Auto    bool // decided by "set for the rest"
}

// Decided reports whether the item has a download choice.
func (it *Item) Decided() bool { return it.Sel != nil && !it.Skipped }

// DisplayTitle returns the best known title.
func (it *Item) DisplayTitle() string {
	if it.Info != nil && it.Info.Title != "" {
		return it.Info.Title
	}
	if it.Title != "" {
		return it.Title
	}
	return it.URL
}

type mode int

const (
	modeAsk  mode = iota // ask for every item, previous choice as default
	modeRest             // apply the template, ask only when it does not fit
)

type screen int

const (
	screenLoading screen = iota
	screenSelect
	screenError
	screenSummary
)

type fetchedMsg struct {
	item *Item
	info *ytdlp.Info
	err  error
}

// App is the bubbletea model.
type App struct {
	runner   ytdlp.Runner
	ctx      context.Context
	sem      chan struct{}
	items    []*Item
	current  int
	mode     mode
	template selection.Selection
	screen   screen
	sel      *selectView
	spinner  spinner.Model
	width    int
	height   int
	errBtn   int
	sumBtn   int
	sumOff   int
	aborted  bool
	finished bool
}

// Run shows the TUI and returns the decided items in order. aborted is
// true when the user left without starting the downloads.
func Run(ctx context.Context, runner ytdlp.Runner, urls []string, jobs int) (plan []*Item, aborted bool, err error) {
	app := newApp(ctx, runner, urls, jobs)
	final, err := tea.NewProgram(app, tea.WithAltScreen(), tea.WithContext(ctx)).Run()
	if err != nil {
		return nil, true, err
	}
	a := final.(*App)
	if a.aborted || !a.finished {
		return nil, true, nil
	}
	for _, it := range a.items {
		if it.Decided() {
			plan = append(plan, it)
		}
	}
	return plan, false, nil
}

func newApp(ctx context.Context, runner ytdlp.Runner, urls []string, jobs int) *App {
	sp := spinner.New(spinner.WithSpinner(spinner.Dot), spinner.WithStyle(styChosen))
	a := &App{runner: runner, ctx: ctx, sem: make(chan struct{}, max(1, jobs)), spinner: sp, width: 80, height: 24}
	for _, u := range urls {
		a.items = append(a.items, &Item{URL: u})
	}
	return a
}

func (a *App) Init() tea.Cmd {
	cmds := []tea.Cmd{a.spinner.Tick}
	for _, it := range a.items {
		cmds = append(cmds, a.fetchCmd(it))
	}
	return tea.Batch(cmds...)
}

func (a *App) fetchCmd(it *Item) tea.Cmd {
	return func() tea.Msg {
		select {
		case a.sem <- struct{}{}:
		case <-a.ctx.Done():
			return fetchedMsg{item: it, err: a.ctx.Err()}
		}
		defer func() { <-a.sem }()
		info, err := a.runner.Fetch(a.ctx, it.URL)
		return fetchedMsg{item: it, info: info, err: err}
	}
}

func (a *App) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		a.width, a.height = msg.Width, msg.Height
		if a.sel != nil {
			a.sel.setSize(a.width, a.height)
		}
		return a, nil
	case spinner.TickMsg:
		var cmd tea.Cmd
		a.spinner, cmd = a.spinner.Update(msg)
		return a, cmd
	case fetchedMsg:
		return a, a.handleFetched(msg)
	case tea.KeyMsg:
		if msg.String() == "ctrl+c" {
			a.aborted = true
			return a, tea.Quit
		}
		switch a.screen {
		case screenSelect:
			return a, a.updateSelect(msg)
		case screenError:
			return a, a.updateError(msg)
		case screenSummary:
			return a, a.updateSummary(msg)
		case screenLoading:
			if msg.String() == "q" {
				a.aborted = true
				return a, tea.Quit
			}
		}
	}
	return a, nil
}

func (a *App) indexOf(it *Item) int {
	for i, x := range a.items {
		if x == it {
			return i
		}
	}
	return -1
}

func (a *App) handleFetched(msg fetchedMsg) tea.Cmd {
	it := msg.item
	idx := a.indexOf(it)
	if idx < 0 {
		return nil
	}
	var cmds []tea.Cmd
	switch {
	case msg.err != nil:
		it.Err = msg.err
	case msg.info.IsPlaylist():
		entries, err := msg.info.PlaylistEntries()
		switch {
		case err != nil:
			it.Err = err
		case it.Depth >= maxPlaylistDepth:
			it.Err = errors.New("playlist nested too deep")
		default:
			var fresh []*Item
			for _, e := range entries {
				n := &Item{URL: e.URL, Title: e.Title, Depth: it.Depth + 1, Info: e.Info}
				fresh = append(fresh, n)
				if n.Info == nil {
					cmds = append(cmds, a.fetchCmd(n))
				}
			}
			rest := append([]*Item{}, a.items[idx+1:]...)
			a.items = append(append(a.items[:idx], fresh...), rest...)
		}
	default:
		it.Info = msg.info
	}
	cmds = append(cmds, a.advance())
	return tea.Batch(cmds...)
}

// lastDecided returns the most recently decided item before current.
func (a *App) lastDecided() *Item {
	for i := a.current - 1; i >= 0; i-- {
		if a.items[i].Decided() {
			return a.items[i]
		}
	}
	return nil
}

// advance moves through the items until one needs user input, all are
// decided, or we have to wait for information.
func (a *App) advance() tea.Cmd {
	for {
		if a.current >= len(a.items) {
			a.screen = screenSummary
			a.sel = nil
			return nil
		}
		it := a.items[a.current]
		if it.Err != nil {
			a.screen = screenError
			a.errBtn = 0
			a.sel = nil
			return nil
		}
		if it.Info == nil {
			a.screen = screenLoading
			a.sel = nil
			return nil
		}
		if a.mode == modeRest {
			r, u := a.template.Resolve(it.Info)
			if !u.Any() {
				t := a.template
				it.Sel, it.Res, it.Auto = &t, r, true
				a.current++
				continue
			}
			note := fmt.Sprintf("The settings used for the rest do not fit this item (%s). Please choose.", u)
			a.openSelect(it, a.template, note)
			return nil
		}
		preset := selection.Default()
		note := ""
		if prev := a.lastDecided(); prev != nil {
			preset = *prev.Sel
			if _, u := preset.Resolve(it.Info); u.Any() {
				note = fmt.Sprintf("Previous choice does not fit this item (%s), using defaults there.", u)
			}
		}
		a.openSelect(it, preset, note)
		return nil
	}
}

func (a *App) isLast() bool { return a.current == len(a.items)-1 }

func (a *App) openSelect(it *Item, preset selection.Selection, note string) {
	var buttons []button
	switch {
	case a.isLast():
		buttons = []button{{label: "Start downloading", act: actStart, key: "n"}, {label: "Abort", act: actAbort, key: "a"}}
	case a.mode == modeRest:
		buttons = []button{{label: "Continue", act: actContinue, key: "n"}, {label: "Set for the rest", act: actRest, key: "r"}, {label: "Abort", act: actAbort, key: "a"}}
	default:
		buttons = []button{{label: "Next", act: actNext, key: "n"}, {label: "Set for the rest", act: actRest, key: "r"}, {label: "Abort", act: actAbort, key: "a"}}
	}
	a.sel = newSelectView(it, a.current+1, len(a.items), preset, buttons, note)
	a.sel.setSize(a.width, a.height)
	a.screen = screenSelect
}

func (a *App) updateSelect(k tea.KeyMsg) tea.Cmd {
	act, ok := a.sel.update(k)
	if !ok {
		return nil
	}
	it := a.sel.item
	switch act {
	case actAbort:
		a.aborted = true
		return tea.Quit
	case actNext, actContinue, actRest, actStart:
		s := a.sel.selection()
		it.Sel = &s
		it.Res, _ = s.Resolve(it.Info)
		it.Auto = false
		if act == actRest {
			a.template, a.mode = s, modeRest
		}
		if act == actStart {
			a.finished = true
			return tea.Quit
		}
		a.current++
		return a.advance()
	}
	return nil
}

func (a *App) updateError(k tea.KeyMsg) tea.Cmd {
	it := a.items[a.current]
	switch k.String() {
	case "left", "h":
		a.errBtn = max(0, a.errBtn-1)
	case "right", "l", "tab":
		a.errBtn = min(2, a.errBtn+1)
	case "q", "a":
		a.aborted = true
		return tea.Quit
	case "r":
		a.errBtn = 0
		return a.activateError(it)
	case "s":
		a.errBtn = 1
		return a.activateError(it)
	case "enter", " ":
		return a.activateError(it)
	}
	return nil
}

func (a *App) activateError(it *Item) tea.Cmd {
	switch a.errBtn {
	case 0: // retry
		it.Err = nil
		a.screen = screenLoading
		return a.fetchCmd(it)
	case 1: // skip
		it.Skipped = true
		a.current++
		return a.advance()
	default:
		a.aborted = true
		return tea.Quit
	}
}

func (a *App) updateSummary(k tea.KeyMsg) tea.Cmd {
	n := 0
	for _, it := range a.items {
		if it.Decided() {
			n++
		}
	}
	switch k.String() {
	case "left", "h":
		a.sumBtn = 0
	case "right", "l", "tab":
		a.sumBtn = 1
	case "up", "k":
		a.sumOff = max(0, a.sumOff-1)
	case "down", "j":
		a.sumOff++
	case "q", "a":
		a.aborted = true
		return tea.Quit
	case "enter", " ", "n":
		if k.String() == "n" {
			a.sumBtn = 0
		}
		if a.sumBtn == 1 || n == 0 {
			a.aborted = true
			return tea.Quit
		}
		a.finished = true
		return tea.Quit
	}
	return nil
}

func (a *App) View() string {
	switch a.screen {
	case screenSelect:
		return a.sel.view()
	case screenError:
		return a.errorView()
	case screenSummary:
		return a.summaryView()
	}
	return a.loadingView()
}

func (a *App) statusLine(i int, it *Item) string {
	var mark, text string
	switch {
	case it.Skipped:
		mark, text = styMuted.Render("–"), styMuted.Render("skipped")
	case it.Err != nil:
		mark, text = styErr.Render("✗"), styErr.Render(it.Err.Error())
	case it.Decided():
		mark, text = styOK.Render("✓"), it.DisplayTitle()
	case it.Info != nil:
		mark, text = styOK.Render("·"), it.DisplayTitle()
	default:
		mark, text = a.spinner.View(), styMuted.Render(it.DisplayTitle())
	}
	return ansi.Truncate(fmt.Sprintf("%s %3d  %s", mark, i+1, text), a.width, "…")
}

func (a *App) loadingView() string {
	var b strings.Builder
	cur := "all items"
	if a.current < len(a.items) {
		cur = fmt.Sprintf("item %d of %d", a.current+1, len(a.items))
	}
	fmt.Fprintf(&b, "%s %s\n\n", styTitle.Render("Fetching information"), styMuted.Render("for "+cur))
	rows := max(1, a.height-6)
	start := max(0, min(a.current-2, len(a.items)-rows))
	for i := start; i < len(a.items) && i < start+rows; i++ {
		b.WriteString(a.statusLine(i, a.items[i]))
		b.WriteString("\n")
	}
	b.WriteString("\n")
	b.WriteString(styHelp.Render("q abort"))
	return b.String()
}

func (a *App) errorView() string {
	it := a.items[a.current]
	var b strings.Builder
	fmt.Fprintf(&b, "%s\n\n", styTitle.Render(fmt.Sprintf("%d/%d  could not get information", a.current+1, len(a.items))))
	b.WriteString(ansi.Truncate(it.URL, a.width, "…"))
	b.WriteString("\n\n")
	b.WriteString(styErr.Render(ansi.Wordwrap(it.Err.Error(), a.width, " ")))
	b.WriteString("\n\n")
	labels := []string{"Retry (r)", "Skip (s)", "Abort (a)"}
	var parts []string
	for i, l := range labels {
		sty := styButton
		if i == a.errBtn {
			sty = styButtonFocused
			if i == 2 {
				sty = styButtonDanger
			}
		}
		parts = append(parts, sty.Render(l))
	}
	b.WriteString(lipgloss.JoinHorizontal(lipgloss.Top, parts...))
	b.WriteString("\n")
	b.WriteString(styHelp.Render("←→ choose · enter confirm"))
	return b.String()
}

func (a *App) summaryView() string {
	var b strings.Builder
	var decided []*Item
	for _, it := range a.items {
		if it.Decided() {
			decided = append(decided, it)
		}
	}
	fmt.Fprintf(&b, "%s %s\n\n", styTitle.Render("Download plan"), styMuted.Render(fmt.Sprintf("%d of %d items", len(decided), len(a.items))))
	rows := max(1, a.height-7)
	if a.width < 120 {
		rows = max(1, rows/2)
	}
	a.sumOff = max(0, min(a.sumOff, max(0, len(a.items)-rows)))
	for i := a.sumOff; i < len(a.items) && i < a.sumOff+rows; i++ {
		it := a.items[i]
		line := a.statusLine(i, it)
		if it.Decided() {
			auto := ""
			if it.Auto {
				auto = styMuted.Render(" (auto)")
			}
			detail := styChosen.Render(it.Res.VideoText()+" · "+it.Res.AudioText()+" · subs "+it.Res.SubsText()) + auto
			if a.width >= 120 {
				line = ansi.Truncate(line+"  "+detail, a.width, "…")
			} else {
				line += "\n" + ansi.Truncate("       "+detail, a.width, "…")
			}
		}
		b.WriteString(line)
		b.WriteString("\n")
	}
	b.WriteString("\n")
	var parts []string
	if len(decided) == 0 {
		parts = append(parts, styButtonDanger.Render("Nothing to download, quit (q)"))
	} else {
		labels := []string{"Start downloading (n)", "Abort (a)"}
		for i, l := range labels {
			sty := styButton
			if i == a.sumBtn {
				sty = styButtonFocused
				if i == 1 {
					sty = styButtonDanger
				}
			}
			parts = append(parts, sty.Render(l))
		}
	}
	b.WriteString(lipgloss.JoinHorizontal(lipgloss.Top, parts...))
	b.WriteString("\n")
	b.WriteString(styHelp.Render("←→ choose · enter confirm · ↑↓ scroll"))
	return b.String()
}
