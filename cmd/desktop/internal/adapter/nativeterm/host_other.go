//go:build !darwin || !ghostty || server

package nativeterm

import "unsafe"

// Available reports whether this build draws native terminals.
func Available() bool { return false }

// Host is a stand-in: New never returns one.
type Host struct{}

// New fails: this build has no native terminals.
func New(unsafe.Pointer, string, func(Event)) (*Host, error) { return nil, ErrUnavailable }

func (*Host) Open(string, Spec) error { return ErrUnavailable }
func (*Host) SetFrame(string, Rect)   {}
func (*Host) Focus(string)            {}
func (*Host) Close(string)            {}
func (*Host) ReloadConfig(string)     {}
func SetOccluders([]Rect)             {}
