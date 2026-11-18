// Command watch runs a command repeatedly and shows its output full
// screen, as watch does on Linux.
package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"runtime/debug"
	"syscall"

	"github.com/bwhaley/watch-macos/watch"
)

// version is set at build time with -ldflags "-X main.version=...".
var version = ""

func main() {
	os.Exit(run())
}

func run() int {
	opts, err := watch.ParseArgs(os.Args[1:], os.Getenv)
	if err != nil {
		fmt.Fprintf(
			os.Stderr,
			"watch: %v\nTry 'watch --help' for more information.\n",
			err,
		)
		return 1
	}
	if opts.Help {
		fmt.Print(watch.Usage)
		return 0
	}
	if opts.Version {
		fmt.Println("watch", buildVersion())
		return 0
	}

	ctx, stop := signal.NotifyContext(
		context.Background(),
		os.Interrupt,
		syscall.SIGTERM,
		syscall.SIGHUP,
	)
	defer stop()
	code, err := watch.Run(ctx, opts, os.Stdin, os.Stdout)
	if err != nil {
		fmt.Fprintf(os.Stderr, "watch: %v\n", err)
		return 1
	}
	return code
}

func buildVersion() string {
	if version != "" {
		return version
	}
	if info, ok := debug.ReadBuildInfo(); ok && info.Main.Version != "" {
		return info.Main.Version
	}
	return "dev"
}
