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
	agentPath  string
	peekPath   string
	directPath string
	sendPath   string
	hubPath    string
	privateCwd string
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
	flags.StringVar(&options.agentPath, "agent-state", "", "path to fm-tui-agent-state.sh")
	flags.StringVar(&options.peekPath, "peek", "", "path to fm-peek.sh")
	flags.StringVar(&options.directPath, "direct", "", "path to fm-tui-direct.sh")
	flags.StringVar(&options.sendPath, "send", "", "path to fm-send.sh")
	flags.StringVar(&options.hubPath, "hub", "", "path to fm-tui-hub.sh")
	flags.StringVar(&options.privateCwd, "private-cwd", "", "allowed working directory for new private Codex threads (default: code root)")
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
	agentPath, err := resolveAdapter(options.agentPath, filepath.Join(root, "bin", "fm-tui-agent-state.sh"))
	if err != nil {
		fmt.Fprintf(stderr, "fm-tui: agent-state adapter: %v\n", err)
		return 1
	}
	directPath, err := resolveAdapter(options.directPath, filepath.Join(root, "bin", "fm-tui-direct.sh"))
	if err != nil {
		fmt.Fprintf(stderr, "fm-tui: direct-session adapter: %v\n", err)
		return 1
	}
	sendPath, err := resolveAdapter(options.sendPath, filepath.Join(root, "bin", "fm-send.sh"))
	if err != nil {
		fmt.Fprintf(stderr, "fm-tui: managed-send adapter: %v\n", err)
		return 1
	}
	hubPath, err := resolveAdapter(options.hubPath, filepath.Join(root, "bin", "fm-tui-hub.sh"))
	if err != nil {
		fmt.Fprintf(stderr, "fm-tui: hub adapter: %v\n", err)
		return 1
	}
	privateCwd := options.privateCwd
	if privateCwd == "" {
		privateCwd = root
	}
	privateCwd, err = resolveDirectory(privateCwd, "private working directory")
	if err != nil {
		fmt.Fprintf(stderr, "fm-tui: %v\n", err)
		return 1
	}

	directSource := firstmate.ShellDirectSessionSource{
		Path:  directPath,
		Home:  home,
		Lines: options.liveRows,
	}

	loader := firstmate.Loader{
		Home: home,
		States: firstmate.ShellStateResolver{
			Path: statePath,
			Home: home,
		},
		Agents: firstmate.ShellAgentStateResolver{
			Path: agentPath,
			Home: home,
		},
		Live: firstmate.ShellLiveReader{
			Path:  peekPath,
			Home:  home,
			Lines: options.liveRows,
		},
		Direct:         directSource,
		DirectCreate:   directSource,
		PrivateWorkdir: privateCwd,
		ManagedSend: firstmate.ShellMessageSender{
			Path: sendPath,
			Home: home,
		},
		DirectSend: firstmate.ShellMessageSender{
			Path:       directPath,
			Home:       home,
			PrefixArgs: []string{"send"},
		},
		Hub: firstmate.ShellHubAdapter{
			Path:  hubPath,
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
	hubTarget, hubErr := loader.LoadHub(ctx)
	var hubHistory []string
	var hubHistoryErr error
	if hubErr == nil {
		hubHistory, hubHistoryErr = loader.LoadHubHistory(ctx, hubTarget)
	}
	hub := tui.HubState{
		Target:     hubTarget,
		History:    hubHistory,
		Err:        hubErr,
		HistoryErr: hubHistoryErr,
	}
	var live []string
	if len(tasks) > 0 {
		live, err = loader.LoadLive(ctx, tasks[0])
		if err != nil {
			live = []string{"[worker capture unavailable] " + err.Error()}
		}
	}

	if options.snapshot {
		fmt.Fprint(stdout, tui.RenderSnapshot(home.Root, hub, tasks, live))
		return 0
	}

	program := tea.NewProgram(tui.NewModel(home.Root, hub, tasks, live, loader))
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

func resolveDirectory(path, label string) (string, error) {
	absolute, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("resolve %s %q: %w", label, path, err)
	}
	info, err := os.Stat(absolute)
	if err != nil {
		return "", fmt.Errorf("%s %q is unavailable: %w", label, absolute, err)
	}
	if !info.IsDir() {
		return "", fmt.Errorf("%s %q is not a directory", label, absolute)
	}
	return absolute, nil
}
