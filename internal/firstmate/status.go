package firstmate

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"strings"
)

// StatusEvent is historical wake-event data. It is never current-state truth.
type StatusEvent struct {
	Verb      string
	Note      string
	Raw       string
	Truncated bool
}

// ReadStatusEvents reads only the newest maxBytes and returns at most maxEvents.
func ReadStatusEvents(path string, maxEvents, maxBytes int) ([]StatusEvent, error) {
	if maxEvents <= 0 || maxBytes <= 0 {
		return nil, fmt.Errorf("status bounds must be positive")
	}
	file, err := os.Open(path)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("open status events %q: %w", path, err)
	}
	defer file.Close()

	info, err := file.Stat()
	if err != nil {
		return nil, fmt.Errorf("stat status events %q: %w", path, err)
	}
	offset := info.Size() - int64(maxBytes)
	byteTruncated := offset > 0
	if offset < 0 {
		offset = 0
	}
	if _, err := file.Seek(offset, io.SeekStart); err != nil {
		return nil, fmt.Errorf("seek status events %q: %w", path, err)
	}
	content, err := io.ReadAll(io.LimitReader(file, int64(maxBytes)))
	if err != nil {
		return nil, fmt.Errorf("read status events %q: %w", path, err)
	}
	if byteTruncated {
		if newline := bytes.IndexByte(content, '\n'); newline >= 0 {
			content = content[newline+1:]
		} else {
			content = nil
		}
	}

	lines := strings.Split(string(content), "\n")
	events := make([]StatusEvent, 0, len(lines))
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		verb, note, found := strings.Cut(line, ":")
		if !found {
			verb = ""
			note = line
		}
		events = append(events, StatusEvent{
			Verb: strings.TrimSpace(verb),
			Note: strings.TrimSpace(note),
			Raw:  line,
		})
	}
	if len(events) > maxEvents {
		events = events[len(events)-maxEvents:]
		byteTruncated = true
	}
	if byteTruncated && len(events) > 0 {
		events[0].Truncated = true
	}
	return events, nil
}
