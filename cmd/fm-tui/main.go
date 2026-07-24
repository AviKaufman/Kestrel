package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/kunchenguid/firstmate/internal/firstmate"
	"github.com/kunchenguid/firstmate/internal/tui"
)

const commandTimeout = 30 * time.Second

type options struct {
	home       string
	root       string
	statePath  string
	peekPath   string
	snapshot   bool
	statusRows int
	liveRows   int
}

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(arguments []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("fm-tui", flag.ContinueOnError)
	flags.SetOutput(stderr)
	var options options
	flags.StringVar(&options.home, "home", "", "Firstmate operational home (default: FM_HOME, then --root)")
	flags.StringVar(&options.root, "root", "", "Firstmate code root containing bin/ (default: FM_ROOT, then current directory)")
	flags.StringVar(&options.statePath, "crew-state", "", "path to fm-crew-state.sh")
	flags.StringVar(&options.peekPath, "peek", "", "path to fm-peek.sh")
	flags.BoolVar(&options.snapshot, "snapshot", false, "print deterministic non-interactive output")
	flags.IntVar(&options.statusRows, "status-lines", 20, "maximum status events per task")
	flags.IntVar(&options.liveRows, "live-lines", 40, "maximum captured worker lines")
	if err := flags.Parse(arguments); err != nil {
		return 2
	}
	if flags.NArg() != 0 {
		fmt.Fprintf(stderr, "fm-tui: unexpected arguments: %v\n", flags.Args())
		return 2
	}
	if options.statusRows <= 0 || options.liveRows <= 0 {
		fmt.Fprintln(stderr, "fm-tui: --status-lines and --live-lines must be positive")
		return 2
	}

	root, err := resolveRoot(options.root)
	if err != nil {
		fmt.Fprintf(stderr, "fm-tui: %v\n", err)
		return 1
	}
	homeArgument := options.home
	if homeArgument == "" && os.Getenv("FM_HOME") == "" {
		homeArgument = root
	}
	home, err := firstmate.ResolveHome(homeArgument)
	if err != nil {
		fmt.Fprintf(stderr, "fm-tui: %v\n", err)
		return 1
	}
	statePath, err := resolveAdapter(options.statePath, filepath.Join(root, "bin", "fm-crew-state.sh"))
	if err != nil {
		fmt.Fprintf(stderr, "fm-tui: current-state adapter: %v\n", err)
		return 1
	}
	peekPath, err := resolveAdapter(options.peekPath, filepath.Join(root, "bin", "fm-peek.sh"))
	if err != nil {
		fmt.Fprintf(stderr, "fm-tui: live adapter: %v\n", err)
		return 1
	}

	loader := firstmate.Loader{
		Home: home,
		States: firstmate.ShellStateResolver{
			Path: statePath,
			Home: home,
		},
		Live: firstmate.ShellLiveReader{
			Path:  peekPath,
			Home:  home,
			Lines: options.liveRows,
		},
		StatusMaxLines: options.statusRows,
		StatusMaxBytes: 64 * 1024,
		ReportMaxBytes: 64 * 1024,
	}

	ctx, cancel := context.WithTimeout(context.Background(), commandTimeout)
	defer cancel()
	tasks, err := loader.Load(ctx)
	if err != nil {
		fmt.Fprintf(stderr, "fm-tui: load tasks: %v\n", err)
		return 1
	}
	var live []string
	if len(tasks) > 0 {
		live, err = loader.LoadLive(ctx, tasks[0].Metadata.ID)
		if err != nil {
			live = []string{"[worker capture unavailable] " + err.Error()}
		}
	}

	if options.snapshot {
		fmt.Fprint(stdout, tui.RenderSnapshot(home.Root, tasks, live))
		return 0
	}

	program := tea.NewProgram(tui.NewModel(home.Root, tasks, live, loader))
	if _, err := program.Run(); err != nil {
		fmt.Fprintf(stderr, "fm-tui: interactive session: %v\n", err)
		return 1
	}
	return 0
}

func resolveRoot(explicit string) (string, error) {
	root := explicit
	if root == "" {
		root = os.Getenv("FM_ROOT")
	}
	if root == "" {
		var err error
		root, err = os.Getwd()
		if err != nil {
			return "", fmt.Errorf("resolve current directory for code root: %w", err)
		}
	}
	absolute, err := filepath.Abs(root)
	if err != nil {
		return "", fmt.Errorf("resolve code root %q: %w", root, err)
	}
	info, err := os.Stat(absolute)
	if err != nil {
		return "", fmt.Errorf("code root %q is unavailable: %w", absolute, err)
	}
	if !info.IsDir() {
		return "", fmt.Errorf("code root %q is not a directory", absolute)
	}
	return absolute, nil
}

func resolveAdapter(explicit, fallback string) (string, error) {
	path := explicit
	if path == "" {
		path = fallback
	}
	absolute, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("resolve %q: %w", path, err)
	}
	info, err := os.Stat(absolute)
	if err != nil {
		return "", fmt.Errorf("%q is unavailable: %w", absolute, err)
	}
	if info.IsDir() {
		return "", fmt.Errorf("%q is a directory", absolute)
	}
	if info.Mode()&0o111 == 0 {
		return "", fmt.Errorf("%q is not executable", absolute)
	}
	return absolute, nil
}
