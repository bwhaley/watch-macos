package watch

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// text returns the rows of g with trailing blanks trimmed.
func text(g grid) []string {
	rows := make([]string, len(g))
	for i, row := range g {
		var b strings.Builder
		for _, c := range row {
			b.WriteRune(c.r)
		}
		rows[i] = strings.TrimRight(b.String(), " ")
	}
	return rows
}

func TestLayout(t *testing.T) {
	tests := []struct {
		name   string
		output string
		noWrap bool
		want   []string
	}{
		{
			name:   "wraps long lines",
			output: "abcdefgh\nij",
			want:   []string{"abcde", "fgh", "ij"},
		},
		{
			name:   "truncates without wrap",
			output: "abcdefgh\nij\nkl\nmn",
			noWrap: true,
			want:   []string{"abcde", "ij", "kl"},
		},
		{
			name:   "cuts at height",
			output: "1\n2\n3\n4\n",
			want:   []string{"1", "2", "3"},
		},
		{
			name:   "expands tabs",
			output: "a\tb\n\tc",
			want:   []string{"a", "b", ""},
		},
		{
			name:   "drops escapes and control characters",
			output: "\x1b[31mred\x1b[0m\r\n\x1b]0;title\a\x07ok\x00",
			want:   []string{"red", "ok", ""},
		},
		{
			name:   "replaces invalid UTF-8",
			output: "a\xffb",
			want:   []string{"a?b", "", ""},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := layout([]byte(tt.output), 5, 3, false, tt.noWrap)
			assert.Equal(t, tt.want, text(g))
		})
	}
}

func TestLayoutTabStops(t *testing.T) {
	g := layout([]byte("a\tb\n\tc"), 20, 2, false, false)
	assert.Equal(t, []string{"a       b", "        c"}, text(g))
}

func TestLayoutColor(t *testing.T) {
	g := layout([]byte("\x1b[1;31mA\x1b[0mB\x1b[38;5;208mC"), 5, 1, true, false)
	assert.Equal(t, style{attrs: 1 << attrBold, fg: "31"}, g[0][0].style)
	assert.Equal(t, style{}, g[0][1].style)
	assert.Equal(t, style{fg: "38;5;208"}, g[0][2].style)
	assert.Equal(t, "\x1b[0;1;31m", g[0][0].style.sgr())
}

func TestStyleApply(t *testing.T) {
	var s style
	s.apply("1;4;32;44")
	assert.Equal(t, style{attrs: 1<<attrBold | 1<<attrUnderline, fg: "32", bg: "44"}, s)
	s.apply("22;24;39")
	assert.Equal(t, style{bg: "44"}, s)
	s.apply("48;2;1;2;3;7")
	assert.Equal(t, style{attrs: 1 << attrReverse, bg: "48;2;1;2;3"}, s)
	s.apply("")
	assert.Equal(t, style{}, s)
}

func TestDifferCycle(t *testing.T) {
	d := differ{mode: DiffCycle}
	assert.Nil(t, d.mark(layout([]byte("abc"), 3, 1, false, false)))

	hl := d.mark(layout([]byte("abd"), 3, 1, false, false))
	assert.Equal(t, [][]bool{{false, false, true}}, hl)

	hl = d.mark(layout([]byte("xbd"), 3, 1, false, false))
	assert.Equal(t, [][]bool{{true, false, false}}, hl)
}

func TestDifferPermanent(t *testing.T) {
	d := differ{mode: DiffPermanent}
	d.mark(layout([]byte("abc"), 3, 1, false, false))
	d.mark(layout([]byte("abd"), 3, 1, false, false))

	hl := d.mark(layout([]byte("xbc"), 3, 1, false, false))
	assert.Equal(t, [][]bool{{true, false, true}}, hl)
}

func TestDifferResetsOnResize(t *testing.T) {
	d := differ{mode: DiffCycle}
	d.mark(layout([]byte("abc"), 3, 1, false, false))
	assert.Nil(t, d.mark(layout([]byte("abd"), 4, 1, false, false)))
}

func TestRenderHighlights(t *testing.T) {
	g := layout([]byte("ab"), 2, 1, false, false)
	var b strings.Builder
	render(&b, g, [][]bool{{false, true}}, 3)
	assert.Equal(t, "\x1b[3;1H\x1b[0ma\x1b[0;7mb\x1b[0m", b.String())
}

func TestHeader(t *testing.T) {
	now := time.Date(2026, 11, 18, 9, 5, 7, 0, time.UTC)
	right := "host: Wed Nov 18 09:05:07 2026"

	got := header(2*time.Second, "date", "host", now, 60)
	assert.Len(t, got, 60)
	assert.True(t, strings.HasPrefix(got, "Every 2.0s: date "))
	assert.True(t, strings.HasSuffix(got, " "+right))

	got = header(
		250*time.Millisecond,
		"a very long command that cannot fit",
		"host",
		now,
		60,
	)
	assert.Equal(t, "Every 0.25s: a very long c... "+right, got)

	got = header(time.Second, "date", "host", now, 20)
	assert.Equal(t, "Every 1.0s: date    ", got)
}

func TestRunCommand(t *testing.T) {
	opts := Options{Command: []string{"echo out; echo err >&2; exit 3"}}
	res := runCommand(context.Background(), opts, 80, 24)
	assert.Equal(t, 3, res.code)
	assert.Equal(t, "out\nerr\n", string(res.output))
}

func TestRunCommandEnv(t *testing.T) {
	opts := Options{Command: []string{"echo $COLUMNS $LINES"}}
	res := runCommand(context.Background(), opts, 100, 30)
	assert.Equal(t, 0, res.code)
	assert.Equal(t, "100 30\n", string(res.output))
}

func TestRunCommandExec(t *testing.T) {
	opts := Options{Exec: true, Command: []string{"echo", "$HOME"}}
	res := runCommand(context.Background(), opts, 80, 24)
	assert.Equal(t, "$HOME\n", string(res.output))

	opts = Options{Exec: true, Command: []string{"no-such-command-xyz"}}
	res = runCommand(context.Background(), opts, 80, 24)
	assert.Equal(t, 127, res.code)
	assert.Contains(t, string(res.output), "no-such-command-xyz")
}

func TestRunCommandCancelKillsGroup(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 200*time.Millisecond)
	defer cancel()
	start := time.Now()
	opts := Options{Command: []string{"sleep 30 & sleep 30"}}
	res := runCommand(ctx, opts, 80, 24)
	require.NotEqual(t, 0, res.code)
	assert.Less(t, time.Since(start), 5*time.Second)
}
