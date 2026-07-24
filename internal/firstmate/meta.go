package firstmate

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Metadata is the bounded task identity and configuration read from state/*.meta.
type Metadata struct {
	ID       string
	Project  string
	Kind     string
	Mode     string
	Yolo     string
	Harness  string
	Model    string
	Effort   string
	Worktree string
	Window   string
	Values   map[string]string
}

// ParseMeta parses key=value records. Repeated fields use the last value, matching
// the shell readers that use tail -1.
func ParseMeta(path string) (Metadata, error) {
	file, err := os.Open(path)
	if err != nil {
		return Metadata{}, fmt.Errorf("open metadata %q: %w", path, err)
	}
	defer file.Close()

	values := make(map[string]string)
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 4096), 1024*1024)
	for scanner.Scan() {
		key, value, found := strings.Cut(scanner.Text(), "=")
		if !found || key == "" {
			continue
		}
		values[key] = value
	}
	if err := scanner.Err(); err != nil {
		return Metadata{}, fmt.Errorf("read metadata %q: %w", path, err)
	}

	id := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
	return Metadata{
		ID:       id,
		Project:  values["project"],
		Kind:     values["kind"],
		Mode:     values["mode"],
		Yolo:     values["yolo"],
		Harness:  values["harness"],
		Model:    values["model"],
		Effort:   values["effort"],
		Worktree: values["worktree"],
		Window:   values["window"],
		Values:   values,
	}, nil
}

// LoadTaskMetadata reads every task metadata file in stable task-id order.
func LoadTaskMetadata(stateDir string) ([]Metadata, error) {
	paths, err := filepath.Glob(filepath.Join(stateDir, "*.meta"))
	if err != nil {
		return nil, fmt.Errorf("list task metadata: %w", err)
	}
	sort.Strings(paths)

	metas := make([]Metadata, 0, len(paths))
	for _, path := range paths {
		meta, err := ParseMeta(path)
		if err != nil {
			return nil, err
		}
		metas = append(metas, meta)
	}
	return metas, nil
}
