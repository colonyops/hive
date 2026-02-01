package jsonfile

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/hay-kot/hive/internal/core/messaging"
)

const (
	debounceDelay   = 50 * time.Millisecond
	eventBufferSize = 100
)

// TopicWatcher watches for changes to topic files using fsnotify.
type TopicWatcher struct {
	topicsDir string
	watcher   *fsnotify.Watcher

	mu          sync.RWMutex
	subscribers map[string][]chan<- messaging.TopicEvent // pattern -> channels
	debounce    map[string]*time.Timer                   // topic -> debounce timer

	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
}

// NewTopicWatcher creates a new watcher for the topics directory.
// The directory is created if it doesn't exist.
func NewTopicWatcher(topicsDir string) (*TopicWatcher, error) {
	if err := os.MkdirAll(topicsDir, 0o755); err != nil {
		return nil, err
	}

	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, err
	}

	if err := watcher.Add(topicsDir); err != nil {
		_ = watcher.Close()
		return nil, err
	}

	ctx, cancel := context.WithCancel(context.Background())
	tw := &TopicWatcher{
		topicsDir:   topicsDir,
		watcher:     watcher,
		subscribers: make(map[string][]chan<- messaging.TopicEvent),
		debounce:    make(map[string]*time.Timer),
		ctx:         ctx,
		cancel:      cancel,
	}

	tw.wg.Add(1)
	go tw.run()

	return tw, nil
}

// Watch returns a channel that receives events when topics matching the pattern change.
func (tw *TopicWatcher) Watch(ctx context.Context, pattern string) (<-chan messaging.TopicEvent, error) {
	ch := make(chan messaging.TopicEvent, eventBufferSize)

	tw.mu.Lock()
	tw.subscribers[pattern] = append(tw.subscribers[pattern], ch)
	tw.mu.Unlock()

	// Handle context cancellation to unsubscribe
	go func() {
		select {
		case <-ctx.Done():
			tw.unsubscribe(pattern, ch)
		case <-tw.ctx.Done():
			// Watcher is closing, channel will be closed by Close()
		}
	}()

	return ch, nil
}

// Close stops watching and closes all subscriber channels.
func (tw *TopicWatcher) Close() error {
	tw.cancel()

	// Stop all debounce timers
	tw.mu.Lock()
	for _, timer := range tw.debounce {
		timer.Stop()
	}

	// Close all subscriber channels
	for _, subs := range tw.subscribers {
		for _, ch := range subs {
			close(ch)
		}
	}
	tw.subscribers = make(map[string][]chan<- messaging.TopicEvent)
	tw.mu.Unlock()

	err := tw.watcher.Close()
	tw.wg.Wait()
	return err
}

// unsubscribe removes a channel from the subscriber list and closes it.
func (tw *TopicWatcher) unsubscribe(pattern string, ch chan<- messaging.TopicEvent) {
	tw.mu.Lock()
	defer tw.mu.Unlock()

	subs := tw.subscribers[pattern]
	for i, sub := range subs {
		if sub == ch {
			tw.subscribers[pattern] = append(subs[:i], subs[i+1:]...)
			close(ch)
			break
		}
	}
	if len(tw.subscribers[pattern]) == 0 {
		delete(tw.subscribers, pattern)
	}
}

// run processes filesystem events from fsnotify.
func (tw *TopicWatcher) run() {
	defer tw.wg.Done()

	for {
		select {
		case <-tw.ctx.Done():
			return
		case event, ok := <-tw.watcher.Events:
			if !ok {
				return
			}
			tw.handleEvent(event)
		case _, ok := <-tw.watcher.Errors:
			if !ok {
				return
			}
			// Log errors in production; for now, ignore
		}
	}
}

// handleEvent processes a single filesystem event.
func (tw *TopicWatcher) handleEvent(event fsnotify.Event) {
	// Only care about writes/creates/renames (file changes)
	if !event.Has(fsnotify.Write) && !event.Has(fsnotify.Create) && !event.Has(fsnotify.Rename) {
		return
	}

	filename := filepath.Base(event.Name)

	// Ignore non-JSON files, temp files, and lock files
	if !strings.HasSuffix(filename, ".json") ||
		strings.HasSuffix(filename, ".tmp") ||
		strings.HasSuffix(filename, ".lock") {
		return
	}

	// Extract topic name from filename
	topic := strings.TrimSuffix(filename, ".json")
	topic = strings.ReplaceAll(topic, "_", "/")

	// Debounce events for this topic
	tw.mu.Lock()
	if timer, exists := tw.debounce[topic]; exists {
		timer.Stop()
	}
	tw.debounce[topic] = time.AfterFunc(debounceDelay, func() {
		tw.notifySubscribers(topic)
	})
	tw.mu.Unlock()
}

// notifySubscribers sends an event to all matching subscribers.
func (tw *TopicWatcher) notifySubscribers(topic string) {
	event := messaging.TopicEvent{
		Topic:     topic,
		Timestamp: time.Now(),
	}

	tw.mu.Lock()
	defer tw.mu.Unlock()

	for pattern, subs := range tw.subscribers {
		if matchesPattern(pattern, topic) {
			for _, ch := range subs {
				select {
				case ch <- event:
				default:
					// Channel full, drop event to prevent blocking
				}
			}
		}
	}

	// Clean up debounce timer
	delete(tw.debounce, topic)
}

// matchesPattern checks if a topic matches a subscription pattern.
func matchesPattern(pattern, topic string) bool {
	// Empty or "*" matches all
	if pattern == "" || pattern == "*" {
		return true
	}

	// Wildcard pattern like "prefix.*"
	if strings.HasSuffix(pattern, ".*") {
		prefix := strings.TrimSuffix(pattern, "*")
		return strings.HasPrefix(topic, prefix)
	}

	// Exact match
	return pattern == topic
}
