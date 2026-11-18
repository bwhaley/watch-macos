# watch-macos

`watch` for macOS, written in Go. It runs a command repeatedly and shows its output full screen, with the options of procps-ng `watch` on Linux.

```sh
watch -n 1 -d 'ls -l'
watch -c -n 0.5 'git -c color.status=always status -s'
watch -g 'curl -s https://example.com/status'
```

## Install

```sh
go install github.com/bwhaley/watch-macos/cmd/watch@latest
```

Or from a checkout:

```sh
make build                  # bin/watch for this machine
make build-darwin           # universal arm64 + amd64 bin/watch, needs lipo
make install PREFIX=$HOME/.local
```

## Options

| Option | Effect |
|---|---|
| `-n, --interval <secs>` | Seconds between runs. Default 2, minimum 0.1. A comma works as the decimal separator. |
| `-d, --differences[=permanent]` | Highlight characters that changed since the last run. `permanent` highlights everything that has changed since the first run. |
| `-c, --color` | Interpret ANSI color and style sequences. Without it they are stripped. |
| `-t, --no-title` | Hide the header. |
| `-w, --no-wrap` | Cut long lines instead of wrapping them. |
| `-b, --beep` | Beep when the command exits non-zero. |
| `-e, --errexit` | Stop when the command exits non-zero. A key press exits with the command's status. |
| `-g, --chgexit` | Exit when the output changes. |
| `-q, --equexit <cycles>` | Exit when the output stays the same for `<cycles>` runs. |
| `-p, --precise` | Start runs every interval, rather than an interval after each run ends. |
| `-x, --exec` | Run the command directly instead of through `sh -c`. |
| `-h, --help`, `-v, --version` | Print help or the version. |

Options stop at the first argument that is not an option, so `watch ps -ef` passes `-ef` to `ps`. Use `--` to run a command whose name starts with `-`.

Press `q` or `Ctrl-C` to quit, and `space` to run the command now.

## Behavior

- The command runs through `sh -c` with its arguments joined by spaces, as on Linux. Quote anything with pipes or globs.
- stdout and stderr appear together. stdin is `/dev/null`.
- The command sees `COLUMNS`, `LINES` and `WATCH_INTERVAL`. Setting `WATCH_INTERVAL` before running `watch` changes the default interval.
- Each run gets its own process group, so quitting also stops anything the command started.
- Resizing the terminal redraws the last output at the new size.

## Limits

- Wide characters, such as CJK and emoji, count as one column, so lines that hold them can misalign.
- Many tools turn off color when stdout is not a terminal. Pass their force-color flag, such as `ls -G` with `CLICOLOR_FORCE=1`, along with `-c`.

## Development

```sh
make lint test
```
