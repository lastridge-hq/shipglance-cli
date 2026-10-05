// Command shipglance reads a ShipGlance workspace from the terminal, scripts
// and coding agents: what is deployed where, what failed, what drifted.
//
// Every run maps to exactly one outcome and exit code (internal/outcome).
// --json (the default when stdout is not a terminal) prints one envelope per
// run: {"outcome", "notice", "data"} where data is the server's /api/v1 body.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/lastridge/shipglance-cli/internal/auth"
	"github.com/lastridge/shipglance-cli/internal/client"
	"github.com/lastridge/shipglance-cli/internal/notice"
	"github.com/lastridge/shipglance-cli/internal/outcome"
	"github.com/lastridge/shipglance-cli/internal/query"
	"github.com/lastridge/shipglance-cli/internal/render"
)

// version is set at release time (-ldflags "-X main.version=…"); a local
// build is "dev" and never sees update notices.
var version = "dev"

// noNoticeEnv turns the terminal update notice off.
const noNoticeEnv = "SHIPGLANCE_NO_UPDATE_NOTIFIER"

// env is everything run touches outside its arguments, injected so tests
// drive the real command tree.
type env struct {
	stdout, stderr io.Writer
	getenv         func(string) string
	now            func() time.Time
	stdoutIsTTY    bool
	cacheDir       string
	http           *http.Client
}

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	cache, err := os.UserCacheDir()
	if err == nil {
		cache = filepath.Join(cache, "shipglance")
	} else {
		cache = ""
	}
	os.Exit(run(ctx, os.Args[1:], env{
		stdout: os.Stdout, stderr: os.Stderr, getenv: os.Getenv, now: time.Now,
		stdoutIsTTY: isTerminal(os.Stdout), cacheDir: cache,
	}))
}

func isTerminal(f *os.File) bool {
	info, err := f.Stat()
	return err == nil && info.Mode()&os.ModeCharDevice != 0
}

const usageText = `shipglance reads a ShipGlance workspace: deploys, failures and drift.

Usage:
  shipglance <command> [flags]

Commands:
  whoami    The workspace, plan and server your token reaches
  status    Every monitored board: status, attention, what is deployed, drift
  drift     What is deployed per stage and whether it is behind its branch
  version   This CLI's version

Flags (every command):
  --json          One JSON envelope on stdout (default when stdout is not a terminal)
  --url URL       Talk to this board URL instead of the one the token names (development)

Flags (status, drift):
  --repo owner/name    Only boards of this repository
  --workflow name      Only this workflow (path or file name, e.g. deploy.yml)

Authentication: export SHIPGLANCE_TOKEN=<token> (Settings → API tokens on your board).

Exit codes: 0 ok · 3 temporary failure, retry · 4 auth · 5 usage · 6 internal or
unknown server response · 7 CLI too old · 9 plan does not include the feature.
`

// run executes one command and returns its exit code. It never panics out:
// a panic is reported as an internal failure (exit 6), so a crash can never
// pass for an ordinary outcome.
func run(ctx context.Context, args []string, e env) (code int) {
	defer func() {
		if recovered := recover(); recovered != nil {
			fmt.Fprintf(e.stderr, "error: internal failure: %v\n", recovered)
			code = outcome.Internal.ExitCode()
		}
	}()
	if len(args) == 0 || args[0] == "help" || args[0] == "-h" || args[0] == "--help" {
		fmt.Fprint(e.stdout, usageText)
		if len(args) == 0 {
			return outcome.Usage.ExitCode()
		}
		return outcome.OK.ExitCode()
	}
	command, rest := args[0], args[1:]
	if command == "--version" {
		command = "version"
	}

	flags := flag.NewFlagSet("shipglance "+command, flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	asJSON := flags.Bool("json", !e.stdoutIsTTY, "")
	baseURL := flags.String("url", "", "")
	var sel query.Selectors
	if command == "status" || command == "drift" {
		flags.StringVar(&sel.Repo, "repo", "", "")
		flags.StringVar(&sel.Workflow, "workflow", "", "")
	}
	usage := func(message string) int {
		return emit(e, *asJSON, query.Fail("invalid_request", message), nil)
	}
	switch command {
	case "whoami", "status", "drift", "version":
	default:
		return usage(fmt.Sprintf("Unknown command %q. Run `shipglance help`.", command))
	}
	if err := flags.Parse(rest); err != nil {
		return usage(fmt.Sprintf("%v. Run `shipglance help`.", err))
	}
	if flags.NArg() > 0 {
		return usage(fmt.Sprintf("Unexpected argument %q. Run `shipglance help`.", flags.Arg(0)))
	}
	if command == "version" {
		return printVersion(e, *asJSON)
	}

	target, err := auth.Resolve(e.getenv(auth.TokenEnv), *baseURL)
	if err != nil {
		var problem *auth.Problem
		if errors.As(err, &problem) {
			return emit(e, *asJSON, query.Fail(problem.Kind, problem.Message), nil)
		}
		return emit(e, *asJSON, query.Fail(outcome.KindInternal, err.Error()), nil)
	}
	c := client.New(target, client.UserAgent(version, runtime.GOOS, runtime.GOARCH, "cli"), e.http)
	var result query.Result
	switch command {
	case "whoami":
		result = query.Whoami(ctx, c, version)
	case "status":
		result = query.Status(ctx, c, version, sel)
	case "drift":
		result = query.Drift(ctx, c, version, sel)
	}
	return emit(e, *asJSON, result, func(w io.Writer) {
		now := e.now()
		switch view := result.View.(type) {
		case *query.WhoamiView:
			render.Whoami(w, view, target.BaseURL)
		case *query.StatusView:
			if command == "drift" {
				render.Drift(w, view, now)
			} else {
				render.Status(w, view, now)
			}
		}
	})
}

// emit writes a result and returns its exit code. In --json mode the
// envelope (notice included) is the whole output; on a terminal the view or
// the problem is written, and the update notice goes to stderr at most once
// a day.
func emit(e env, asJSON bool, result query.Result, text func(io.Writer)) int {
	if asJSON {
		if err := render.JSON(e.stdout, result); err != nil {
			fmt.Fprintf(e.stderr, "error: %v\n", err)
			return outcome.Internal.ExitCode()
		}
		return result.Outcome.ExitCode()
	}
	if result.Outcome == outcome.OK && text != nil {
		text(e.stdout)
	} else {
		render.Problem(e.stderr, result)
	}
	if result.Notice != nil && strings.TrimSpace(e.getenv(noNoticeEnv)) == "" {
		throttle := notice.Throttle{Dir: e.cacheDir, Now: e.now}
		if throttle.Due() {
			render.Notice(e.stderr, result.Notice)
			throttle.Shown()
		}
	}
	return result.Outcome.ExitCode()
}

func printVersion(e env, asJSON bool) int {
	if asJSON {
		data := fmt.Sprintf(`{"version":%q,"os":%q,"arch":%q}`, version, runtime.GOOS, runtime.GOARCH)
		return emit(e, true, query.Result{Outcome: outcome.OK, Data: []byte(data)}, nil)
	}
	fmt.Fprintf(e.stdout, "shipglance %s (%s/%s)\n", version, runtime.GOOS, runtime.GOARCH)
	return outcome.OK.ExitCode()
}
