package daemon

import (
	"context"
	"sync"
	"time"

	"github.com/colonyops/hive/internal/hive/gitstatus"
)

// gitStatusTTL is how long one read answers every client. A read costs
// several git processes per session, and each client polls on its own.
const gitStatusTTL = 5 * time.Second

type gitStatusCache struct {
	read func(ctx context.Context, ids []string) (map[string]gitstatus.Status, error)
	now  func() time.Time

	mu      sync.Mutex
	entries map[string]cachedGitStatus
}

type cachedGitStatus struct {
	status gitstatus.Status
	readAt time.Time
}

func newGitStatusCache(read func(ctx context.Context, ids []string) (map[string]gitstatus.Status, error)) *gitStatusCache {
	return &gitStatusCache{read: read, now: time.Now, entries: map[string]cachedGitStatus{}}
}

// get answers fresh entries from the cache and reads the rest in one batch.
// Two clients that miss at the same moment both read; the cost is one extra
// read, which does not justify a wait between them. The read outlives the
// caller's context because its result is shared: one client hanging up must
// not cache a cancelled read for the others. The read has its own time limit.
func (c *gitStatusCache) get(ctx context.Context, ids []string) (map[string]gitstatus.Status, error) {
	out := make(map[string]gitstatus.Status, len(ids))
	var missing []string
	now := c.now()

	c.mu.Lock()
	for id, entry := range c.entries {
		if now.Sub(entry.readAt) > gitStatusTTL {
			delete(c.entries, id)
		}
	}
	for _, id := range ids {
		if entry, ok := c.entries[id]; ok {
			out[id] = entry.status
		} else {
			missing = append(missing, id)
		}
	}
	c.mu.Unlock()

	if len(missing) == 0 {
		return out, nil
	}
	read, err := c.read(context.WithoutCancel(ctx), missing)
	if err != nil {
		return nil, err
	}
	readAt := c.now()
	c.mu.Lock()
	for id, status := range read {
		c.entries[id] = cachedGitStatus{status: status, readAt: readAt}
		out[id] = status
	}
	c.mu.Unlock()
	return out, nil
}
