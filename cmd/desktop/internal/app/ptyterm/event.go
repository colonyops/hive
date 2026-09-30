package ptyterm

// Event is one item on a terminal's outbound stream.
type Event interface{ isEvent() }

// Output is a terminal's bytes, straight off the PTY.
type Output struct {
	Data []byte
}

// Exited reports that the process behind the PTY is gone, with a reason meant
// to be read by a user. Nothing follows it on the stream.
type Exited struct {
	Reason string
}

func (Output) isEvent() {}
func (Exited) isEvent() {}
