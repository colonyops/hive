package todo

import "fmt"

// ValidateTransition checks whether a status change is allowed.
func ValidateTransition(from, to Status) error {
	allowed := map[Status][]Status{
		StatusPending:      {StatusAcknowledged, StatusCompleted, StatusDismissed},
		StatusAcknowledged: {StatusCompleted, StatusDismissed},
		StatusCompleted:    {StatusAcknowledged},
		StatusDismissed:    {StatusAcknowledged},
	}

	for _, candidate := range allowed[from] {
		if candidate == to {
			return nil
		}
	}

	return fmt.Errorf("invalid transition from %q to %q", from, to)
}
