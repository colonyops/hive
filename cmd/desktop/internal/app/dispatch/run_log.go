package dispatch

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"strings"
	"sync"
	"time"

	"github.com/rs/zerolog"

	"github.com/colonyops/hive/cmd/desktop/internal/app/data/stores"
)

const (
	RunLogStdout = "stdout"
	RunLogStderr = "stderr"
	RunLogSystem = "system"

	maxRunLogAttemptBytes = 8 * 1024 * 1024
	maxRunLogLineBytes    = 16 * 1024
	runLogFlushInterval   = 250 * time.Millisecond
	runLogWriteTimeout    = 3 * time.Second
)

// RunLogSink persists the lines of one attempt of an output command.
type RunLogSink interface {
	AppendLog(ctx context.Context, commandID, attempt int64, lines []stores.NewActionRunLogLine) error
}

// RunLog collects what one attempt of an output command did: the output of
// any process it ran and the system lines an executor writes to describe
// steps that have no process, such as creating a session. Lines reach the
// sink in batches while the attempt runs, so a viewer can follow it live.
//
// A nil *RunLog discards everything, which is what an execution without a
// durable command (a terminal-target action) gets.
type RunLog struct {
	sink      RunLogSink
	commandID int64
	attempt   int64
	logger    zerolog.Logger
	now       func() time.Time

	mu        sync.Mutex
	pending   []stores.NewActionRunLogLine
	partial   map[string]*bytes.Buffer
	bytes     int
	truncated bool
	failed    bool

	stop      chan struct{}
	done      chan struct{}
	closeOnce sync.Once
}

func newRunLog(ctx context.Context, sink RunLogSink, commandID, attempt int64, logger zerolog.Logger) *RunLog {
	if sink == nil {
		return nil
	}
	l := &RunLog{
		sink: sink, commandID: commandID, attempt: attempt, logger: logger, now: time.Now,
		partial: map[string]*bytes.Buffer{RunLogStdout: {}, RunLogStderr: {}},
		stop:    make(chan struct{}), done: make(chan struct{}),
	}
	go l.flushLoop(context.WithoutCancel(ctx))
	return l
}

type runLogKey struct{}

func WithRunLog(ctx context.Context, l *RunLog) context.Context {
	return context.WithValue(ctx, runLogKey{}, l)
}

func RunLogFrom(ctx context.Context) *RunLog {
	l, _ := ctx.Value(runLogKey{}).(*RunLog)
	return l
}

func (l *RunLog) Stdout() io.Writer { return l.stream(RunLogStdout) }
func (l *RunLog) Stderr() io.Writer { return l.stream(RunLogStderr) }

func (l *RunLog) stream(name string) io.Writer {
	if l == nil {
		return io.Discard
	}
	return runLogStream{log: l, name: name}
}

// Systemf writes one system line per line of the formatted text.
func (l *RunLog) Systemf(format string, args ...any) {
	if l == nil {
		return
	}
	text := strings.TrimRight(fmt.Sprintf(format, args...), "\n")
	l.mu.Lock()
	defer l.mu.Unlock()
	for line := range strings.SplitSeq(text, "\n") {
		l.appendLocked(RunLogSystem, line)
	}
}

// Close writes any unterminated output and every pending line, and returns
// once they are stored. Later writes are dropped. The writes outlive a
// cancelled ctx: a cancelled run still needs its last lines stored.
func (l *RunLog) Close(ctx context.Context) {
	if l == nil {
		return
	}
	l.closeOnce.Do(func() {
		close(l.stop)
		<-l.done
		l.mu.Lock()
		for _, name := range []string{RunLogStdout, RunLogStderr} {
			if buf := l.partial[name]; buf.Len() > 0 {
				l.appendLocked(name, buf.String())
				buf.Reset()
			}
		}
		l.mu.Unlock()
		l.flush(context.WithoutCancel(ctx))
	})
}

type runLogStream struct {
	log  *RunLog
	name string
}

func (s runLogStream) Write(p []byte) (int, error) {
	l := s.log
	l.mu.Lock()
	defer l.mu.Unlock()
	buf := l.partial[s.name]
	buf.Write(p)
	for {
		data := buf.Bytes()
		i := bytes.IndexByte(data, '\n')
		if i < 0 {
			if buf.Len() >= maxRunLogLineBytes {
				l.appendLocked(s.name, buf.String())
				buf.Reset()
			}
			break
		}
		l.appendLocked(s.name, string(data[:i]))
		buf.Next(i + 1)
	}
	return len(p), nil
}

func (l *RunLog) appendLocked(stream, text string) {
	if l.truncated {
		return
	}
	text = strings.TrimSuffix(text, "\r")
	if l.bytes+len(text) > maxRunLogAttemptBytes {
		l.truncated = true
		l.pending = append(l.pending, stores.NewActionRunLogLine{
			Stream: RunLogSystem, At: l.now(),
			Text: fmt.Sprintf("Output truncated: this attempt wrote more than %d KiB", maxRunLogAttemptBytes/1024),
		})
		return
	}
	l.bytes += len(text)
	l.pending = append(l.pending, stores.NewActionRunLogLine{Stream: stream, Text: text, At: l.now()})
}

func (l *RunLog) flushLoop(ctx context.Context) {
	defer close(l.done)
	ticker := time.NewTicker(runLogFlushInterval)
	defer ticker.Stop()
	for {
		select {
		case <-l.stop:
			return
		case <-ticker.C:
			l.flush(ctx)
		}
	}
}

func (l *RunLog) flush(ctx context.Context) {
	l.mu.Lock()
	lines := l.pending
	l.pending = nil
	l.mu.Unlock()
	if len(lines) == 0 {
		return
	}
	ctx, cancel := context.WithTimeout(ctx, runLogWriteTimeout)
	defer cancel()
	if err := l.sink.AppendLog(ctx, l.commandID, l.attempt, lines); err != nil {
		// A log write failure must not fail the action, and repeating the
		// warning for every batch would bury the first one.
		l.mu.Lock()
		first := !l.failed
		l.failed = true
		l.mu.Unlock()
		if first {
			l.logger.Warn().Err(err).Int64("command_id", l.commandID).Msg("action run log: write failed")
		}
	}
}

// indentLines sets text such as a rendered message apart from the system
// lines around it.
func indentLines(text string) string {
	lines := strings.Split(strings.TrimRight(text, "\n"), "\n")
	for i, line := range lines {
		lines[i] = "  " + line
	}
	return strings.Join(lines, "\n")
}
