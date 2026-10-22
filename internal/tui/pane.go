package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
)

// paneItem is one row of a pane.
type paneItem struct {
	label  string
	header bool // section heading, cannot be selected
	check  bool // checkbox instead of radio button
}

// pane is a scrollable, filterable list with radio buttons and checkboxes.
type pane struct {
	title     string
	items     []paneItem
	radio     int // index of the selected radio item, -1 for none
	checks    map[int]bool
	visible   []int // indices into items after filtering
	cursor    int   // position in visible
	offset    int   // first visible row shown
	filter    string
	filtering bool
	width     int // outer width including border
	height    int // outer height including border
	focused   bool
	disabled  string // when set, the pane only shows this text
}

func newPane(title string, items []paneItem) pane {
	p := pane{title: title, items: items, radio: -1, checks: map[int]bool{}}
	p.applyFilter()
	return p
}

// setDisabled makes the pane show only a note; empty text re-enables it.
func (p *pane) setDisabled(text string) { p.disabled = text }

func (p *pane) applyFilter() {
	p.visible = p.visible[:0]
	needle := strings.ToLower(p.filter)
	for i, it := range p.items {
		if needle != "" && (it.header || !strings.Contains(strings.ToLower(it.label), needle)) {
			continue
		}
		p.visible = append(p.visible, i)
	}
	if p.cursor >= len(p.visible) {
		p.cursor = max(0, len(p.visible)-1)
	}
	p.skipHeader(1)
}

// skipHeader moves the cursor off a header row in the given direction.
func (p *pane) skipHeader(dir int) {
	for n := 0; n < len(p.visible); n++ {
		if p.cursor < 0 || p.cursor >= len(p.visible) {
			break
		}
		if !p.items[p.visible[p.cursor]].header {
			return
		}
		p.cursor += dir
	}
	// Only headers in that direction, try the other way.
	p.cursor = max(0, min(p.cursor, len(p.visible)-1))
	for p.cursor > 0 && p.items[p.visible[p.cursor]].header {
		p.cursor--
	}
}

func (p *pane) move(delta int) {
	if len(p.visible) == 0 {
		return
	}
	p.cursor = max(0, min(len(p.visible)-1, p.cursor+delta))
	dir := 1
	if delta < 0 {
		dir = -1
	}
	p.skipHeader(dir)
}

// moveTo puts the cursor on an item index, if it is visible.
func (p *pane) moveTo(idx int) {
	for v, i := range p.visible {
		if i == idx {
			p.cursor = v
			return
		}
	}
}

// current returns the item index under the cursor.
func (p *pane) current() (int, bool) {
	if len(p.visible) == 0 || p.cursor < 0 || p.cursor >= len(p.visible) {
		return 0, false
	}
	return p.visible[p.cursor], true
}

// toggle activates the item under the cursor and returns its index.
func (p *pane) toggle() (int, bool) {
	idx, ok := p.current()
	if !ok || p.items[idx].header {
		return 0, false
	}
	if p.items[idx].check {
		p.checks[idx] = !p.checks[idx]
	} else {
		p.radio = idx
	}
	return idx, true
}

func (p *pane) rows() int {
	n := p.height - 2 - 1 // border, title
	if p.filter != "" || p.filtering {
		n--
	}
	return max(1, n)
}

// handleKey processes a key and reports whether it was consumed.
func (p *pane) handleKey(k tea.KeyMsg) bool {
	if p.disabled != "" {
		return false
	}
	if p.filtering {
		switch k.Type {
		case tea.KeyEsc:
			p.filter, p.filtering = "", false
			p.applyFilter()
		case tea.KeyEnter:
			p.filtering = false
		case tea.KeyBackspace:
			if r := []rune(p.filter); len(r) > 0 {
				p.filter = string(r[:len(r)-1])
				p.applyFilter()
			}
		case tea.KeyUp:
			p.move(-1)
		case tea.KeyDown:
			p.move(1)
		case tea.KeyRunes:
			p.filter += string(k.Runes)
			p.applyFilter()
		case tea.KeySpace:
			p.filter += " "
			p.applyFilter()
		default:
			return false
		}
		return true
	}
	switch k.String() {
	case "up", "k":
		p.move(-1)
	case "down", "j":
		p.move(1)
	case "pgup", "ctrl+u":
		p.move(-p.rows())
	case "pgdown", "ctrl+d":
		p.move(p.rows())
	case "home", "g":
		p.cursor = 0
		p.skipHeader(1)
	case "end", "G":
		p.cursor = len(p.visible) - 1
		p.skipHeader(-1)
	case "/":
		p.filtering = true
	case "esc":
		if p.filter == "" {
			return false
		}
		p.filter = ""
		p.applyFilter()
	default:
		return false
	}
	return true
}

// chosenLabels returns the labels of the selected radio and checked items.
func (p *pane) chosenLabels() []string {
	var out []string
	if p.radio >= 0 && p.radio < len(p.items) {
		out = append(out, p.items[p.radio].label)
	}
	for i, it := range p.items {
		if it.check && p.checks[i] {
			out = append(out, it.label)
		}
	}
	return out
}

func (p *pane) view() string {
	inner := p.width - 2
	sty := styPane
	if p.focused {
		sty = styPaneFocused
	}
	rows := p.rows()
	if p.cursor < p.offset {
		p.offset = p.cursor
	}
	if p.cursor >= p.offset+rows {
		p.offset = p.cursor - rows + 1
	}
	p.offset = max(0, min(p.offset, max(0, len(p.visible)-rows)))

	var b strings.Builder
	title := styHeading.Render(p.title)
	if p.filter != "" || p.filtering {
		f := "/" + p.filter
		if p.filtering {
			f += "▏"
		}
		title += " " + styChosen.Render(f)
	}
	if p.disabled == "" && len(p.visible) > rows {
		pos := fmt.Sprintf("%d-%d/%d", p.offset+1, min(len(p.visible), p.offset+rows), len(p.visible))
		if pad := inner - ansi.StringWidth(title) - len(pos) - 1; pad > 0 {
			title += strings.Repeat(" ", pad) + styMuted.Render(pos)
		}
	}
	b.WriteString(ansi.Truncate(title, inner, "…"))
	b.WriteString("\n")

	if p.disabled != "" {
		b.WriteString(styMuted.Render(ansi.Wordwrap(p.disabled, inner, " ")))
		return sty.Width(inner).Height(p.height - 2).Render(b.String())
	}

	for r := 0; r < rows; r++ {
		v := p.offset + r
		if v >= len(p.visible) {
			break
		}
		idx := p.visible[v]
		it := p.items[idx]
		var line string
		switch {
		case it.header:
			line = styMuted.Render(ansi.Truncate(it.label, inner, "…"))
		default:
			mark := "○ "
			chosen := false
			if it.check {
				mark = "☐ "
				if p.checks[idx] {
					mark, chosen = "☑ ", true
				}
			} else if p.radio == idx {
				mark, chosen = "◉ ", true
			}
			text := ansi.Truncate(it.label, inner-2, "…")
			switch {
			case v == p.cursor && p.focused:
				line = styCursor.Width(inner).Render(mark + text)
			case v == p.cursor:
				line = styCursorNF.Render(mark + text)
			case chosen:
				line = styChosen.Render(mark + text)
			default:
				line = mark + text
			}
		}
		b.WriteString(line)
		if r < rows-1 {
			b.WriteString("\n")
		}
	}
	return sty.Width(inner).Height(p.height - 2).Render(b.String())
}

// collapsed renders a one line summary used in the narrow layout.
func (p *pane) collapsed(width int) string {
	chosen := p.chosenLabels()
	text := "–"
	if p.disabled != "" {
		text = p.disabled
	} else if len(chosen) > 0 {
		text = strings.Join(chosen, ", ")
	}
	line := styHeading.Render(p.title+":") + " " + styChosen.Render(text)
	return ansi.Truncate(line, width, "…")
}
