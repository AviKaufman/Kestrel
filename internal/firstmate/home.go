package firstmate

import (
	"fmt"
	"os"
	"path/filepath"
)

// Home contains the read-only paths used by the TUI for one Firstmate home.
type Home struct {
	Root     string
	StateDir string
	DataDir  string
}

// ResolveHome applies --home, FM_HOME, then the current directory.
func ResolveHome(explicit string) (Home, error) {
	root := explicit
	if root == "" {
		root = os.Getenv("FM_HOME")
	}
	if root == "" {
		var err error
		root, err = os.Getwd()
		if err != nil {
			return Home{}, fmt.Errorf("resolve current directory for FM_HOME: %w", err)
		}
	}

	absolute, err := filepath.Abs(root)
	if err != nil {
		return Home{}, fmt.Errorf("resolve FM_HOME %q: %w", root, err)
	}
	info, err := os.Stat(absolute)
	if err != nil {
		return Home{}, fmt.Errorf("FM_HOME %q is unavailable: %w", absolute, err)
	}
	if !info.IsDir() {
		return Home{}, fmt.Errorf("FM_HOME %q is not a directory", absolute)
	}

	stateDir := filepath.Join(absolute, "state")
	stateInfo, err := os.Stat(stateDir)
	if err != nil {
		return Home{}, fmt.Errorf("FM_HOME %q has no readable state directory: %w", absolute, err)
	}
	if !stateInfo.IsDir() {
		return Home{}, fmt.Errorf("FM_HOME %q state path is not a directory", absolute)
	}

	return Home{
		Root:     absolute,
		StateDir: stateDir,
		DataDir:  filepath.Join(absolute, "data"),
	}, nil
}
