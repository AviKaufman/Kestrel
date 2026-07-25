package firstmate

import (
	"context"
	"fmt"
	"regexp"
	"strings"
)

var supervisorTargetPattern = regexp.MustCompile(`^[A-Za-z0-9%:@._-]{1,256}$`)

// HubTarget is the validated existing primary supervisor endpoint.
type HubTarget struct {
	Backend string
	Target  string
}

// HubAdapter resolves and sends to the current primary supervisor.
type HubAdapter interface {
	Resolve(context.Context) (HubTarget, error)
	Read(context.Context, HubTarget) ([]string, error)
	Send(context.Context, HubTarget, string) error
}

// LoadHubHistory reads bounded primary-supervisor conversation from the hub adapter.
func (loader Loader) LoadHubHistory(ctx context.Context, target HubTarget) ([]string, error) {
	if loader.Hub == nil {
		return nil, fmt.Errorf("hub adapter is required")
	}
	if !validHubTarget(target) {
		return nil, fmt.Errorf("invalid Firstmate hub target %#v", target)
	}
	return loader.Hub.Read(ctx, target)
}

// ParseHubTarget parses one backend<TAB>target record.
func ParseHubTarget(output string) (HubTarget, error) {
	line := strings.TrimSuffix(output, "\n")
	if line == "" || strings.Contains(line, "\n") {
		return HubTarget{}, fmt.Errorf("hub adapter returned an invalid record: %q", line)
	}
	backend, target, found := strings.Cut(line, "\t")
	if !found || !validHubTarget(HubTarget{Backend: backend, Target: target}) {
		return HubTarget{}, fmt.Errorf("hub adapter returned an invalid record: %q", line)
	}
	return HubTarget{Backend: backend, Target: target}, nil
}

func validHubTarget(target HubTarget) bool {
	switch target.Backend {
	case "tmux", "herdr":
	default:
		return false
	}
	return supervisorTargetPattern.MatchString(target.Target)
}

// LoadHub resolves the current primary supervisor without fabricating fallback state.
func (loader Loader) LoadHub(ctx context.Context) (HubTarget, error) {
	if loader.Hub == nil {
		return HubTarget{}, fmt.Errorf("hub adapter is required")
	}
	return loader.Hub.Resolve(ctx)
}

// SendHub validates a message and routes it through the primary supervisor adapter.
func (loader Loader) SendHub(ctx context.Context, target HubTarget, message string) error {
	if loader.Hub == nil {
		return fmt.Errorf("hub adapter is required")
	}
	if !validHubTarget(target) {
		return fmt.Errorf("invalid Firstmate hub target %#v", target)
	}
	if strings.TrimSpace(message) == "" {
		return fmt.Errorf("message must not be empty")
	}
	if len([]byte(message)) > MaxMessageBytes {
		return fmt.Errorf("message exceeds %d-byte limit", MaxMessageBytes)
	}
	return loader.Hub.Send(ctx, target, message)
}
