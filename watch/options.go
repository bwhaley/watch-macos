package watch

import (
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"
)

const (
	defaultInterval = 2 * time.Second
	minInterval     = 100 * time.Millisecond
)

// DiffMode selects which changes the screen highlights.
type DiffMode int

const (
	// DiffOff highlights nothing.
	DiffOff DiffMode = 0
	// DiffCycle highlights what changed since the previous run.
	DiffCycle DiffMode = 1
	// DiffPermanent highlights every cell that has changed since the first
	// run, and keeps it highlighted.
	DiffPermanent DiffMode = 2
)

func (m DiffMode) String() string {
	switch m {
	case DiffOff:
		return "off"
	case DiffCycle:
		return "cycle"
	case DiffPermanent:
		return "permanent"
	default:
		return fmt.Sprintf("DiffMode(%d)", int(m))
	}
}

// Valid reports whether m is one of the defined modes.
func (m DiffMode) Valid() bool {
	switch m {
	case DiffOff, DiffCycle, DiffPermanent:
		return true
	default:
		return false
	}
}

// Options is the parsed command line.
type Options struct {
	Interval    time.Duration
	Differences DiffMode
	NoTitle     bool
	Beep        bool
	ErrExit     bool
	ChgExit     bool
	// EqExit is the number of consecutive unchanged runs after which watch
	// exits. Zero disables it.
	EqExit  int
	Exec    bool
	Precise bool
	Color   bool
	NoWrap  bool
	Help    bool
	Version bool
	Command []string
}

// CommandString is the command as the title shows it.
func (o Options) CommandString() string {
	return strings.Join(o.Command, " ")
}

type argKind int

const (
	argNone argKind = iota
	argRequired
	argOptional
)

type optSpec struct {
	short byte
	long  string
	arg   argKind
	apply func(o *Options, val string, hasVal bool) error
}

func setFlag(field func(o *Options) *bool) func(*Options, string, bool) error {
	return func(o *Options, _ string, _ bool) error {
		*field(o) = true
		return nil
	}
}

var optSpecs = []optSpec{
	{
		short: 'b',
		long:  "beep",
		apply: setFlag(func(o *Options) *bool { return &o.Beep }),
	},
	{
		short: 'c',
		long:  "color",
		apply: setFlag(func(o *Options) *bool { return &o.Color }),
	},
	{
		short: 'd',
		long:  "differences",
		arg:   argOptional,
		apply: func(o *Options, val string, hasVal bool) error {
			if !hasVal {
				o.Differences = DiffCycle
				return nil
			}
			if val != "permanent" {
				return fmt.Errorf(
					"invalid argument %q for --differences, want \"permanent\"",
					val,
				)
			}
			o.Differences = DiffPermanent
			return nil
		},
	},
	{
		short: 'e',
		long:  "errexit",
		apply: setFlag(func(o *Options) *bool { return &o.ErrExit }),
	},
	{
		short: 'g',
		long:  "chgexit",
		apply: setFlag(func(o *Options) *bool { return &o.ChgExit }),
	},
	{
		short: 'h',
		long:  "help",
		apply: setFlag(func(o *Options) *bool { return &o.Help }),
	},
	{
		short: 'n',
		long:  "interval",
		arg:   argRequired,
		apply: func(o *Options, val string, _ bool) error {
			d, err := parseInterval(val)
			if err != nil {
				return err
			}
			o.Interval = d
			return nil
		},
	},
	{
		short: 'p',
		long:  "precise",
		apply: setFlag(func(o *Options) *bool { return &o.Precise }),
	},
	{
		short: 'q',
		long:  "equexit",
		arg:   argRequired,
		apply: func(o *Options, val string, _ bool) error {
			n, err := strconv.Atoi(val)
			if err != nil || n < 1 {
				return fmt.Errorf("invalid number of cycles %q", val)
			}
			o.EqExit = n
			return nil
		},
	},
	{
		short: 't',
		long:  "no-title",
		apply: setFlag(func(o *Options) *bool { return &o.NoTitle }),
	},
	{
		short: 'v',
		long:  "version",
		apply: setFlag(func(o *Options) *bool { return &o.Version }),
	},
	{
		short: 'w',
		long:  "no-wrap",
		apply: setFlag(func(o *Options) *bool { return &o.NoWrap }),
	},
	{
		short: 'x',
		long:  "exec",
		apply: setFlag(func(o *Options) *bool { return &o.Exec }),
	},
}

// ParseArgs parses args, which exclude the program name. Options stop at the
// first argument that is not an option, as in procps watch, so the command
// keeps its own flags. getenv supplies WATCH_INTERVAL.
func ParseArgs(args []string, getenv func(string) string) (Options, error) {
	opts := Options{Interval: defaultInterval}
	if env := getenv("WATCH_INTERVAL"); env != "" {
		d, err := parseInterval(env)
		if err != nil {
			return Options{}, fmt.Errorf("WATCH_INTERVAL: %w", err)
		}
		opts.Interval = d
	}

	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch {
		case arg == "--":
			opts.Command = args[i+1:]
			return finish(opts)
		case strings.HasPrefix(arg, "--"):
			next, err := parseLong(&opts, args, i)
			if err != nil {
				return Options{}, err
			}
			i = next
		case len(arg) > 1 && arg[0] == '-':
			next, err := parseShort(&opts, args, i)
			if err != nil {
				return Options{}, err
			}
			i = next
		default:
			opts.Command = args[i:]
			return finish(opts)
		}
	}
	return finish(opts)
}

func finish(opts Options) (Options, error) {
	if opts.Help || opts.Version {
		return opts, nil
	}
	if len(opts.Command) == 0 {
		return Options{}, errors.New("no command given")
	}
	return opts, nil
}

// parseLong applies args[i] and returns the index of the last argument it
// consumed.
func parseLong(opts *Options, args []string, i int) (int, error) {
	name, val, hasVal := strings.Cut(args[i][2:], "=")
	spec, err := lookupLong(name)
	if err != nil {
		return 0, err
	}
	switch spec.arg {
	case argNone:
		if hasVal {
			return 0, fmt.Errorf(
				"option '--%s' doesn't allow an argument",
				spec.long,
			)
		}
	case argRequired:
		if !hasVal {
			if i+1 >= len(args) {
				return 0, fmt.Errorf(
					"option '--%s' requires an argument",
					spec.long,
				)
			}
			i++
			val = args[i]
			hasVal = true
		}
	case argOptional:
	}
	return i, spec.apply(opts, val, hasVal)
}

// lookupLong matches an exact name first, then a unique prefix, as
// getopt_long does.
func lookupLong(name string) (optSpec, error) {
	var matches []optSpec
	for _, spec := range optSpecs {
		if spec.long == name {
			return spec, nil
		}
		if strings.HasPrefix(spec.long, name) {
			matches = append(matches, spec)
		}
	}
	switch len(matches) {
	case 0:
		return optSpec{}, fmt.Errorf("unrecognized option '--%s'", name)
	case 1:
		return matches[0], nil
	default:
		return optSpec{}, fmt.Errorf("option '--%s' is ambiguous", name)
	}
}

// parseShort applies a cluster of short options such as -dtn1 and returns
// the index of the last argument it consumed. -d takes no argument in short
// form, so it can share a cluster.
func parseShort(opts *Options, args []string, i int) (int, error) {
	cluster := args[i][1:]
	for j := 0; j < len(cluster); j++ {
		spec, ok := lookupShort(cluster[j])
		if !ok {
			return 0, fmt.Errorf("invalid option -- '%c'", cluster[j])
		}
		if spec.arg != argRequired {
			if err := spec.apply(opts, "", false); err != nil {
				return 0, err
			}
			continue
		}
		val := cluster[j+1:]
		if val == "" {
			if i+1 >= len(args) {
				return 0, fmt.Errorf(
					"option requires an argument -- '%c'",
					spec.short,
				)
			}
			i++
			val = args[i]
		}
		return i, spec.apply(opts, val, true)
	}
	return i, nil
}

func lookupShort(c byte) (optSpec, bool) {
	for _, spec := range optSpecs {
		if spec.short == c {
			return spec, true
		}
	}
	return optSpec{}, false
}

// parseInterval reads seconds, with a comma accepted as the decimal
// separator, and raises anything below 0.1s to 0.1s.
func parseInterval(s string) (time.Duration, error) {
	secs, err := strconv.ParseFloat(strings.ReplaceAll(s, ",", "."), 64)
	if err != nil ||
		math.IsNaN(secs) ||
		math.IsInf(secs, 0) ||
		secs < 0 ||
		secs > math.MaxInt32 {
		return 0, fmt.Errorf("failed to parse argument: '%s'", s)
	}
	return max(time.Duration(secs*float64(time.Second)), minInterval), nil
}

// Usage is the --help text.
const Usage = `Usage:
 watch [options] command

Options:
  -b, --beep             beep if command has a non-zero exit
  -c, --color            interpret ANSI color and style sequences
  -d, --differences[=permanent]
                         highlight changes between updates
  -e, --errexit          exit if command has a non-zero exit
  -g, --chgexit          exit when output from command changes
  -n, --interval <secs>  seconds to wait between updates
  -p, --precise          attempt run command in precise intervals
  -q, --equexit <cycles> exit when output from command does not change
  -t, --no-title         turn off header
  -w, --no-wrap          turn off line wrapping
  -x, --exec             pass command to exec instead of "sh -c"

 -h, --help     display this help and exit
 -v, --version  output version information and exit

Keys:
  space  run the command now
  q      quit

The WATCH_INTERVAL environment variable sets the default interval.
`
