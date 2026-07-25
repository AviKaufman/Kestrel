package firstmate

import (
	"context"
	"fmt"
	"regexp"
	"sort"
	"strings"
)

// Ownership identifies the authority that owns a worker session.
type Ownership string

const (
	FirstmateManaged Ownership = "Firstmate managed"
	CaptainPrivate   Ownership = "Captain private / Direct Codex"
)

var directTargetPattern = regexp.MustCompile(`^[A-Za-z0-9_.-]+:[A-Za-z0-9_.-]+\.[0-9]+$`)

// AgentStateResolver returns the recovery-grade live agent state for a task.
type AgentStateResolver interface {
	ResolveAgentState(context.Context, string) (string, error)
}

// DirectSession is a live tmux Codex pane not claimed by Firstmate metadata.
type DirectSession struct {
	Target  string
	Project string
}

// DirectSessionSource discovers and captures captain-private Codex sessions.
type DirectSessionSource interface {
	Discover(context.Context, []Metadata) ([]DirectSession, error)
	Read(context.Context, string) ([]string, error)
}

// ParseDirectSessions parses target<TAB>project records in stable target order.
func ParseDirectSessions(output string) ([]DirectSession, error) {
	trimmed := strings.TrimRight(output, "\n")
	if trimmed == "" {
		return nil, nil
	}
	seen := make(map[string]struct{})
	sessions := make([]DirectSession, 0)
	for _, line := range strings.Split(trimmed, "\n") {
		target, project, found := strings.Cut(line, "\t")
		if !found || !directTargetPattern.MatchString(target) || strings.ContainsAny(project, "\r\n\t") {
			return nil, fmt.Errorf("direct-session adapter returned an invalid record: %q", line)
		}
		if _, found := seen[target]; found {
			return nil, fmt.Errorf("direct-session adapter returned duplicate target %q", target)
		}
		seen[target] = struct{}{}
		sessions = append(sessions, DirectSession{Target: target, Project: project})
	}
	sort.Slice(sessions, func(left, right int) bool {
		return sessions[left].Target < sessions[right].Target
	})
	return sessions, nil
}

func isTerminalState(state string) bool {
	switch strings.ToLower(strings.TrimSpace(state)) {
	case "done", "failed", "cancelled", "canceled", "completed", "terminated":
		return true
	default:
		return false
	}
}
