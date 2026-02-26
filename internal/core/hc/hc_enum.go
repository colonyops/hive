package hc

import (
	"errors"
	"fmt"
)

// --- ItemType ---

const (
	// ItemTypeEpic is an ItemType of type epic.
	ItemTypeEpic ItemType = "epic"
	// ItemTypeTask is an ItemType of type task.
	ItemTypeTask ItemType = "task"
)

var ErrInvalidItemType = errors.New("not a valid ItemType")

// String implements the Stringer interface.
func (x ItemType) String() string {
	return string(x)
}

// IsValid provides a quick way to determine if the typed value is
// part of the allowed enumerated values.
func (x ItemType) IsValid() bool {
	_, err := ParseItemType(string(x))
	return err == nil
}

var _ItemTypeValue = map[string]ItemType{
	"epic": ItemTypeEpic,
	"task": ItemTypeTask,
}

// ParseItemType attempts to convert a string to an ItemType.
func ParseItemType(name string) (ItemType, error) {
	if x, ok := _ItemTypeValue[name]; ok {
		return x, nil
	}
	return ItemType(""), fmt.Errorf("%s is %w", name, ErrInvalidItemType)
}

// --- Status ---

const (
	// StatusOpen is a Status of type open.
	StatusOpen Status = "open"
	// StatusInProgress is a Status of type in_progress.
	StatusInProgress Status = "in_progress"
	// StatusDone is a Status of type done.
	StatusDone Status = "done"
	// StatusCancelled is a Status of type cancelled.
	StatusCancelled Status = "cancelled"
)

var ErrInvalidStatus = errors.New("not a valid Status")

// String implements the Stringer interface.
func (x Status) String() string {
	return string(x)
}

// IsValid provides a quick way to determine if the typed value is
// part of the allowed enumerated values.
func (x Status) IsValid() bool {
	_, err := ParseStatus(string(x))
	return err == nil
}

var _StatusValue = map[string]Status{
	"open":        StatusOpen,
	"in_progress": StatusInProgress,
	"done":        StatusDone,
	"cancelled":   StatusCancelled,
}

// ParseStatus attempts to convert a string to a Status.
func ParseStatus(name string) (Status, error) {
	if x, ok := _StatusValue[name]; ok {
		return x, nil
	}
	return Status(""), fmt.Errorf("%s is %w", name, ErrInvalidStatus)
}

// --- ActivityType ---

const (
	// ActivityTypeUpdate is an ActivityType of type update.
	ActivityTypeUpdate ActivityType = "update"
	// ActivityTypeComment is an ActivityType of type comment.
	ActivityTypeComment ActivityType = "comment"
	// ActivityTypeCheckpoint is an ActivityType of type checkpoint.
	ActivityTypeCheckpoint ActivityType = "checkpoint"
	// ActivityTypeStatusChange is an ActivityType of type status_change.
	ActivityTypeStatusChange ActivityType = "status_change"
)

var ErrInvalidActivityType = errors.New("not a valid ActivityType")

// String implements the Stringer interface.
func (x ActivityType) String() string {
	return string(x)
}

// IsValid provides a quick way to determine if the typed value is
// part of the allowed enumerated values.
func (x ActivityType) IsValid() bool {
	_, err := ParseActivityType(string(x))
	return err == nil
}

var _ActivityTypeValue = map[string]ActivityType{
	"update":        ActivityTypeUpdate,
	"comment":       ActivityTypeComment,
	"checkpoint":    ActivityTypeCheckpoint,
	"status_change": ActivityTypeStatusChange,
}

// ParseActivityType attempts to convert a string to an ActivityType.
func ParseActivityType(name string) (ActivityType, error) {
	if x, ok := _ActivityTypeValue[name]; ok {
		return x, nil
	}
	return ActivityType(""), fmt.Errorf("%s is %w", name, ErrInvalidActivityType)
}
