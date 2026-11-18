package watch

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func noEnv(string) string { return "" }

func TestParseArgs(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want Options
	}{
		{
			name: "defaults",
			args: []string{"date"},
			want: Options{Interval: 2 * time.Second, Command: []string{"date"}},
		},
		{
			name: "separate interval",
			args: []string{"-n", "5", "date"},
			want: Options{Interval: 5 * time.Second, Command: []string{"date"}},
		},
		{
			name: "attached interval with comma decimal",
			args: []string{"-n0,5", "date"},
			want: Options{
				Interval: 500 * time.Millisecond,
				Command:  []string{"date"},
			},
		},
		{
			name: "interval floor",
			args: []string{"--interval=0.01", "date"},
			want: Options{
				Interval: 100 * time.Millisecond,
				Command:  []string{"date"},
			},
		},
		{
			name: "cluster ending in an argument",
			args: []string{"-dtbn1", "ls", "-l"},
			want: Options{
				Interval:    time.Second,
				Differences: DiffCycle,
				NoTitle:     true,
				Beep:        true,
				Command:     []string{"ls", "-l"},
			},
		},
		{
			name: "long options and prefix",
			args: []string{
				"--differences=permanent",
				"--prec",
				"--equexit",
				"3",
				"--color",
				"--no-wrap",
				"-x",
				"ls",
			},
			want: Options{
				Interval:    2 * time.Second,
				Differences: DiffPermanent,
				Precise:     true,
				EqExit:      3,
				Color:       true,
				NoWrap:      true,
				Exec:        true,
				Command:     []string{"ls"},
			},
		},
		{
			name: "options stop at the command",
			args: []string{"-g", "ps", "-e", "--forest"},
			want: Options{
				Interval: 2 * time.Second,
				ChgExit:  true,
				Command:  []string{"ps", "-e", "--forest"},
			},
		},
		{
			name: "double dash",
			args: []string{"-e", "--", "-weird-command"},
			want: Options{
				Interval: 2 * time.Second,
				ErrExit:  true,
				Command:  []string{"-weird-command"},
			},
		},
		{
			name: "help needs no command",
			args: []string{"--help"},
			want: Options{Interval: 2 * time.Second, Help: true},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseArgs(tt.args, noEnv)
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestParseArgsEnvInterval(t *testing.T) {
	env := func(k string) string {
		if k == "WATCH_INTERVAL" {
			return "1.5"
		}
		return ""
	}

	got, err := ParseArgs([]string{"date"}, env)
	require.NoError(t, err)
	assert.Equal(t, 1500*time.Millisecond, got.Interval)

	got, err = ParseArgs([]string{"-n", "3", "date"}, env)
	require.NoError(t, err)
	assert.Equal(t, 3*time.Second, got.Interval)
}

func TestParseArgsErrors(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want string
	}{
		{"no command", []string{"-d"}, "no command given"},
		{"unknown short", []string{"-z", "date"}, "invalid option -- 'z'"},
		{"unknown long", []string{"--zzz", "date"}, "unrecognized option '--zzz'"},
		{"ambiguous long", []string{"--e", "date"}, "option '--e' is ambiguous"},
		{"missing argument", []string{"-n"}, "option requires an argument -- 'n'"},
		{"bad interval", []string{"-n", "abc", "date"}, "failed to parse argument: 'abc'"},
		{"negative interval", []string{"-n", "-1", "date"}, "failed to parse argument: '-1'"},
		{"bad differences", []string{"--differences=x", "date"}, `invalid argument "x"`},
		{"flag with value", []string{"--beep=1", "date"}, "doesn't allow an argument"},
		{"bad cycles", []string{"-q", "0", "date"}, `invalid number of cycles "0"`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := ParseArgs(tt.args, noEnv)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.want)
		})
	}
}
