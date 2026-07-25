package firstmate

import (
	"context"
	"fmt"
	"regexp"
	"strings"
)

// MaxMessageBytes bounds one composer submission before an adapter is invoked.
const MaxMessageBytes = 4096

var managedTargetPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]*$`)

// MessageSender sends one validated message to one validated adapter target.
type MessageSender interface {
	Send(context.Context, string, string) error
}

// Send validates ownership, target, and message before explicit adapter routing.
func (loader Loader) Send(ctx context.Context, task Task, message string) error {
	if strings.TrimSpace(message) == "" {
		return fmt.Errorf("message must not be empty")
	}
	if len([]byte(message)) > MaxMessageBytes {
		return fmt.Errorf("message exceeds %d-byte limit", MaxMessageBytes)
	}

	switch task.Ownership {
	case FirstmateManaged:
		if loader.ManagedSend == nil {
			return fmt.Errorf("managed sender is required")
		}
		if task.Target != task.Metadata.ID || !managedTargetPattern.MatchString(task.Target) {
			return fmt.Errorf("invalid Firstmate-managed target %q", task.Target)
		}
		return loader.ManagedSend.Send(ctx, task.Target, message)
	case CaptainPrivate:
		if loader.DirectSend == nil {
			return fmt.Errorf("direct-session sender is required")
		}
		if task.Metadata.ID != "" && task.Metadata.ID != task.Target {
			return fmt.Errorf("direct-session identity does not match target %q", task.Target)
		}
		if !directTargetPattern.MatchString(task.Target) {
			return fmt.Errorf("invalid direct-session target %q", task.Target)
		}
		return loader.DirectSend.Send(ctx, task.Target, message)
	default:
		return fmt.Errorf("unsupported worker ownership %q", task.Ownership)
	}
}
