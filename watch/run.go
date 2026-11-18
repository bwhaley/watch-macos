package watch

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"golang.org/x/term"
)

const (
	enterScreen = "\x1b[?1049h\x1b[?25l\x1b[?7l\x1b[2J"
	leaveScreen = "\x1b[0m\x1b[?7h\x1b[?25h\x1b[?1049l"
	syncStart   = "\x1b[?2026h"
	syncEnd     = "\x1b[?2026l"
	ctrlC       = 0x03
)

// result is one run of the command.
type result struct {
	output []byte
	code   int
}

// Run watches opts.Command on the terminal at stdout until the user quits,
// ctx ends, or an exit option fires. It returns the exit status for the
// process.
func Run(
	ctx context.Context,
	opts Options,
	stdin *os.File,
	stdout *os.File,
) (int, error) {
	outFd := int(stdout.Fd())
	if !term.IsTerminal(outFd) {
		return 0, errors.New("standard output is not a terminal")
	}
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	keys := make(chan byte, 1)
	refresh := make(chan struct{}, 1)
	inFd := int(stdin.Fd())
	interactive := term.IsTerminal(inFd)
	if interactive {
		state, err := term.MakeRaw(inFd)
		if err != nil {
			return 0, fmt.Errorf("set terminal to raw mode: %w", err)
		}
		defer func() { _ = term.Restore(inFd, state) }()
		go readKeys(stdin, cancel, keys, refresh)
	}

	winch := make(chan os.Signal, 1)
	signal.Notify(winch, syscall.SIGWINCH)
	defer signal.Stop(winch)

	if _, err := io.WriteString(stdout, enterScreen); err != nil {
		return 0, fmt.Errorf("write to terminal: %w", err)
	}
	defer func() { _, _ = io.WriteString(stdout, leaveScreen) }()

	host, _ := os.Hostname()
	w := &watcher{
		opts:        opts,
		out:         stdout,
		outFd:       outFd,
		interactive: interactive,
		host:        host,
		differ:      differ{mode: opts.Differences},
	}
	return w.loop(ctx, keys, refresh, winch)
}

// readKeys turns key presses into actions until stdin closes. q and ^C
// quit, space asks for a run now, and every key lands on keys.
func readKeys(
	stdin io.Reader,
	quit context.CancelFunc,
	keys chan<- byte,
	refresh chan<- struct{},
) {
	buf := make([]byte, 64)
	for {
		n, err := stdin.Read(buf)
		if err != nil {
			return
		}
		for _, k := range buf[:n] {
			switch k {
			case 'q', ctrlC:
				quit()
			case ' ':
				select {
				case refresh <- struct{}{}:
				default:
				}
			}
			select {
			case keys <- k:
			default:
			}
		}
	}
}

type watcher struct {
	opts        Options
	out         io.Writer
	outFd       int
	interactive bool
	host        string
	differ      differ
	last        *result
	lastGrid    grid
	highlight   [][]bool
}

func (w *watcher) loop(
	ctx context.Context,
	keys <-chan byte,
	refresh <-chan struct{},
	winch <-chan os.Signal,
) (int, error) {
	unchanged := 0
	next := time.Now()
	for {
		width, height := w.size()
		res := runCommand(ctx, w.opts, width, height)
		if ctx.Err() != nil {
			return 0, nil
		}
		prev := w.last
		w.last = &res
		if err := w.draw(true); err != nil {
			return 0, err
		}

		if res.code != 0 && w.opts.Beep {
			if _, err := io.WriteString(w.out, "\a"); err != nil {
				return 0, fmt.Errorf("write to terminal: %w", err)
			}
		}
		if res.code != 0 && w.opts.ErrExit {
			return w.errExit(ctx, keys, res.code)
		}
		if prev != nil {
			if !bytes.Equal(prev.output, res.output) {
				if w.opts.ChgExit {
					return 0, nil
				}
				unchanged = 0
			} else if unchanged++; w.opts.EqExit > 0 &&
				unchanged >= w.opts.EqExit {
				return 0, nil
			}
		}

		if w.opts.Precise {
			next = next.Add(w.opts.Interval)
			if now := time.Now(); next.Before(now) {
				next = now
			}
		} else {
			next = time.Now().Add(w.opts.Interval)
		}
		if err := w.wait(ctx, next, refresh, winch); err != nil {
			return 0, err
		}
		if ctx.Err() != nil {
			return 0, nil
		}
	}
}

// wait sleeps until next, a refresh, or the end of ctx, and redraws the
// last output on each resize.
func (w *watcher) wait(
	ctx context.Context,
	next time.Time,
	refresh <-chan struct{},
	winch <-chan os.Signal,
) error {
	timer := time.NewTimer(time.Until(next))
	defer timer.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-timer.C:
			return nil
		case <-refresh:
			return nil
		case <-winch:
			if err := w.draw(false); err != nil {
				return err
			}
		}
	}
}

// errExit leaves the failed output on screen until a key press, then
// returns the command's status.
func (w *watcher) errExit(
	ctx context.Context,
	keys <-chan byte,
	code int,
) (int, error) {
	width, height := w.size()
	msg := "command exit with a non-zero status, press a key to exit"
	if _, err := fmt.Fprintf(
		w.out,
		"\x1b[%d;1H\x1b[0;7m%s\x1b[0m",
		height,
		string([]rune(msg)[:min(len(msg), width)]),
	); err != nil {
		return 0, fmt.Errorf("write to terminal: %w", err)
	}
	if !w.interactive {
		return code, nil
	}
	select {
	case <-keys:
	default:
	}
	select {
	case <-ctx.Done():
	case <-keys:
	}
	return code, nil
}

func (w *watcher) size() (int, int) {
	width, height, err := term.GetSize(w.outFd)
	if err != nil || width <= 0 || height <= 0 {
		return 80, 24
	}
	return width, height
}

// draw lays out the last output at the current terminal size and writes
// the whole screen in one write. fresh marks output from a new run, which
// is the only output the differ compares.
func (w *watcher) draw(fresh bool) error {
	if w.last == nil {
		return nil
	}
	width, height := w.size()
	top := 1
	var b strings.Builder
	b.WriteString(syncStart)
	if !w.opts.NoTitle {
		fmt.Fprintf(
			&b,
			"\x1b[1;1H\x1b[0m%s\x1b[2;1H\x1b[2K",
			header(
				w.opts.Interval,
				w.opts.CommandString(),
				w.host,
				time.Now(),
				width,
			),
		)
		top = 3
	}
	g := layout(
		w.last.output,
		width,
		max(height-top+1, 0),
		w.opts.Color,
		w.opts.NoWrap,
	)
	if fresh ||
		len(g) != len(w.lastGrid) ||
		g.width() != w.lastGrid.width() {
		w.highlight = w.differ.mark(g)
	}
	w.lastGrid = g
	render(&b, g, w.highlight, top)
	b.WriteString(syncEnd)
	if _, err := io.WriteString(w.out, b.String()); err != nil {
		return fmt.Errorf("write to terminal: %w", err)
	}
	return nil
}

// runCommand runs the command once with stdout and stderr captured
// together. COLUMNS and LINES carry the terminal size.
func runCommand(
	ctx context.Context,
	opts Options,
	width int,
	height int,
) result {
	var cmd *exec.Cmd
	if opts.Exec {
		cmd = exec.CommandContext(ctx, opts.Command[0], opts.Command[1:]...)
	} else {
		cmd = exec.CommandContext(ctx, "sh", "-c", opts.CommandString())
	}
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	cmd.Env = append(
		os.Environ(),
		"COLUMNS="+strconv.Itoa(width),
		"LINES="+strconv.Itoa(height),
		"WATCH_INTERVAL="+strconv.FormatFloat(opts.Interval.Seconds(), 'f', -1, 64),
	)
	// The command gets its own process group so a quit also stops the
	// processes it started.
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error {
		return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
	}
	cmd.WaitDelay = time.Second

	err := cmd.Run()
	var exitErr *exec.ExitError
	switch {
	case err == nil:
		return result{output: out.Bytes()}
	case errors.As(err, &exitErr):
		return result{output: out.Bytes(), code: max(exitErr.ExitCode(), 1)}
	default:
		fmt.Fprintf(&out, "watch: %v\n", err)
		return result{output: out.Bytes(), code: 127}
	}
}
