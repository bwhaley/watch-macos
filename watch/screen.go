package watch

import (
	"fmt"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

const tabWidth = 8

type cell struct {
	r     rune
	style style
}

var blank = cell{r: ' '}

// grid is the visible output, height rows of width cells, padded with
// blanks.
type grid [][]cell

func newGrid(width int, height int) grid {
	g := make(grid, height)
	for row := range g {
		g[row] = make([]cell, width)
		for col := range g[row] {
			g[row][col] = blank
		}
	}
	return g
}

func (g grid) width() int {
	if len(g) == 0 {
		return 0
	}
	return len(g[0])
}

// layout places output on a width by height grid. Lines longer than width
// wrap unless noWrap is set. Escape sequences set cell styles when color is
// set and are dropped otherwise, as are other control characters.
func layout(
	output []byte,
	width int,
	height int,
	color bool,
	noWrap bool,
) grid {
	g := newGrid(width, height)
	if width <= 0 {
		return g
	}
	var cur style
	row, col := 0, 0
	newline := func() {
		row++
		col = 0
	}
	put := func(r rune) {
		if col >= width {
			if noWrap {
				return
			}
			newline()
		}
		if row >= height {
			return
		}
		g[row][col] = cell{r: r, style: cur}
		col++
	}

	for i := 0; i < len(output) && row < height; {
		r, size := utf8.DecodeRune(output[i:])
		switch {
		case r == '\x1b':
			params, final, n := readEscape(output[i:])
			if color && final == 'm' {
				cur.apply(params)
			}
			i += n
			continue
		case r == '\n':
			newline()
		case r == '\t':
			if col >= width && !noWrap {
				newline()
			}
			if row >= height {
				break
			}
			stop := min((col/tabWidth+1)*tabWidth, width)
			for ; col < stop; col++ {
				g[row][col] = cell{r: ' ', style: cur}
			}
		case r == utf8.RuneError && size <= 1:
			put('?')
		case unicode.IsPrint(r):
			put(r)
		}
		i += size
	}
	return g
}

// readEscape reads the escape sequence at the start of b. For a CSI
// sequence it returns the parameters and the final byte. For any other
// sequence final is 0. n is the length of the sequence in bytes.
func readEscape(b []byte) (params string, final byte, n int) {
	if len(b) < 2 {
		return "", 0, len(b)
	}
	switch b[1] {
	case '[':
		for i := 2; i < len(b); i++ {
			if b[i] >= 0x40 && b[i] <= 0x7e {
				return string(b[2:i]), b[i], i + 1
			}
		}
		return "", 0, len(b)
	case ']', 'P', '_', '^':
		// OSC, DCS, APC and PM run to BEL or ESC \.
		for i := 2; i < len(b); i++ {
			if b[i] == '\a' {
				return "", 0, i + 1
			}
			if b[i] == '\x1b' && i+1 < len(b) && b[i+1] == '\\' {
				return "", 0, i + 2
			}
		}
		return "", 0, len(b)
	default:
		return "", 0, 2
	}
}

// differ tracks which cells to highlight across runs.
type differ struct {
	mode    DiffMode
	prev    grid
	changed [][]bool // DiffPermanent only: cells that differed from first
}

// mark returns the cells of g to highlight and records g for the next run.
// A grid of a new size resets the history, so a resize highlights nothing.
func (d *differ) mark(g grid) [][]bool {
	if d.mode == DiffOff {
		return nil
	}
	if len(d.prev) != len(g) || d.prev.width() != g.width() {
		d.prev = g
		d.changed = nil
		return nil
	}
	hl := make([][]bool, len(g))
	for row := range g {
		hl[row] = make([]bool, len(g[row]))
		for col := range g[row] {
			hl[row][col] = g[row][col].r != d.prev[row][col].r
		}
	}
	if d.mode == DiffCycle {
		d.prev = g
		return hl
	}
	// DiffPermanent keeps the first grid as prev and ORs into changed.
	if d.changed == nil {
		d.changed = hl
		return hl
	}
	for row := range hl {
		for col := range hl[row] {
			d.changed[row][col] = d.changed[row][col] || hl[row][col]
		}
	}
	return d.changed
}

// render draws g starting at screen row top (1-based), reversing the
// highlighted cells.
func render(b *strings.Builder, g grid, hl [][]bool, top int) {
	for row := range g {
		fmt.Fprintf(b, "\x1b[%d;1H", top+row)
		last := ""
		for col, c := range g[row] {
			s := c.style
			if hl != nil && hl[row][col] {
				s = s.with(attrReverse)
			}
			if seq := s.sgr(); seq != last {
				b.WriteString(seq)
				last = seq
			}
			b.WriteRune(c.r)
		}
	}
	b.WriteString("\x1b[0m")
}

// header is the title line: the interval and command on the left, the host
// and time on the right. The command is cut to fit width.
func header(
	interval time.Duration,
	command string,
	host string,
	now time.Time,
	width int,
) string {
	left := []rune(fmt.Sprintf("Every %s: %s", formatInterval(interval), command))
	right := []rune(fmt.Sprintf("%s: %s", host, now.Format(time.ANSIC)))

	// Keep a space between the halves. Drop the right half before the
	// interval when the screen is too narrow for both.
	room := width - len(right) - 1
	if room < len("Every 2.0s: ...") {
		room = width
		right = nil
	}
	if len(left) > room {
		left = append(left[:max(room-3, 0)], []rune("...")...)
		left = left[:min(len(left), max(room, 0))]
	}
	pad := max(width-len(left)-len(right), 0)
	return string(left) + strings.Repeat(" ", pad) + string(right)
}

// formatInterval prints seconds with at least one decimal, as in "2.0s" or
// "0.25s".
func formatInterval(d time.Duration) string {
	s := fmt.Sprintf("%g", d.Seconds())
	if !strings.ContainsAny(s, ".e") {
		s += ".0"
	}
	return s + "s"
}
