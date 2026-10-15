package userdb

import (
	"container/list"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/rs/zerolog"

	"github.com/colonyops/hive/cmd/drive-server/internal/core"
	"github.com/colonyops/hive/pkg/logutils"
)

const minSweepInterval = 5 * time.Millisecond

type entry struct {
	id       string
	db       *UserDB
	leases   int
	lastUsed time.Time
	elem     *list.Element
	removing bool
}

// Pool is an LRU of open user databases. A handle with a live lease is never
// closed by eviction or idle close.
type Pool struct {
	log     zerolog.Logger
	dir     string
	maxOpen int
	idle    time.Duration

	mu      sync.Mutex
	changed *sync.Cond // leases dropped or a removal finished
	entries map[string]*entry
	lru     *list.List // front is most recently used
	closed  bool

	stop      chan struct{}
	done      chan struct{}
	closeOnce sync.Once
}

// NewPool opens no files. maxOpen bounds the cached handles (leased handles
// can exceed it), and handles unused for idle are closed; idle <= 0 disables
// idle close.
func NewPool(log zerolog.Logger, dir string, maxOpen int, idle time.Duration) *Pool {
	p := &Pool{
		log:     logutils.Component(log, "userdb"),
		dir:     dir,
		maxOpen: max(maxOpen, 1),
		idle:    idle,
		entries: map[string]*entry{},
		lru:     list.New(),
		stop:    make(chan struct{}),
		done:    make(chan struct{}),
	}
	p.changed = sync.NewCond(&p.mu)
	if idle > 0 {
		go p.sweepLoop(max(idle/2, minSweepInterval))
	} else {
		close(p.done)
	}
	return p
}

// Open returns userID's database, creating an empty one on first use. The
// caller must call release exactly when done; the handle stays open until then.
func (p *Pool) Open(ctx context.Context, userID string) (*UserDB, func(), error) {
	if err := validateID(userID); err != nil {
		return nil, nil, err
	}

	p.mu.Lock()
	defer p.mu.Unlock()
	for {
		if p.closed {
			return nil, nil, errors.New("userdb: pool is closed")
		}
		if e := p.entries[userID]; e != nil && e.removing {
			p.changed.Wait()
			continue
		}
		break
	}

	e := p.entries[userID]
	if e == nil {
		if err := os.MkdirAll(p.dir, 0o700); err != nil {
			return nil, nil, fmt.Errorf("userdb: create users dir: %w", err)
		}
		udb, err := openUserDB(ctx, p.path(userID))
		if err != nil {
			return nil, nil, err
		}
		p.evictLocked()
		e = &entry{id: userID, db: udb}
		e.elem = p.lru.PushFront(e)
		p.entries[userID] = e
	} else {
		p.lru.MoveToFront(e.elem)
	}
	e.leases++
	e.lastUsed = time.Now()

	var once sync.Once
	release := func() {
		once.Do(func() {
			p.mu.Lock()
			defer p.mu.Unlock()
			e.leases--
			e.lastUsed = time.Now()
			p.changed.Broadcast()
		})
	}
	return e.db, release, nil
}

// Remove waits for the user's leases to drain, closes the handle, and deletes
// the database and its WAL files.
func (p *Pool) Remove(_ context.Context, userID string) error {
	if err := validateID(userID); err != nil {
		return err
	}

	p.mu.Lock()
	defer p.mu.Unlock()
	for {
		e := p.entries[userID]
		if e == nil {
			break
		}
		e.removing = true
		if e.leases == 0 {
			p.logClose(p.dropLocked(e), e.id)
			break
		}
		if p.closed {
			return errors.New("userdb: pool is closed")
		}
		p.changed.Wait()
	}

	var errs []error
	for _, suffix := range []string{"", "-wal", "-shm"} {
		if err := os.Remove(p.path(userID) + suffix); err != nil && !errors.Is(err, os.ErrNotExist) {
			errs = append(errs, err)
		}
	}
	p.changed.Broadcast()
	if err := errors.Join(errs...); err != nil {
		return fmt.Errorf("userdb: remove %s: %w", userID, err)
	}
	return nil
}

// Close stops idle close and closes every handle, leased or not. Callers stop
// issuing requests before they close the pool.
func (p *Pool) Close() error {
	var err error
	p.closeOnce.Do(func() {
		close(p.stop)
		<-p.done

		p.mu.Lock()
		defer p.mu.Unlock()
		p.closed = true
		var errs []error
		for _, e := range p.entries {
			errs = append(errs, p.dropLocked(e))
		}
		p.changed.Broadcast()
		err = errors.Join(errs...)
	})
	return err
}

// Stores adapts the pool to the core port, which hands out DocumentStores.
func (p *Pool) Stores() core.UserStores { return stores{p} }

type stores struct{ p *Pool }

func (s stores) Open(ctx context.Context, userID string) (core.DocumentStore, func(), error) {
	udb, release, err := s.p.Open(ctx, userID)
	if err != nil {
		return nil, nil, err
	}
	return udb, release, nil
}

func (s stores) Remove(ctx context.Context, userID string) error { return s.p.Remove(ctx, userID) }

func (p *Pool) path(userID string) string { return filepath.Join(p.dir, userID+".db") }

// evictLocked makes room for one more handle by closing the least recently
// used unleased ones.
func (p *Pool) evictLocked() {
	for el := p.lru.Back(); el != nil && len(p.entries) >= p.maxOpen; {
		e := el.Value.(*entry)
		el = el.Prev()
		if e.leases == 0 && !e.removing {
			p.logClose(p.dropLocked(e), e.id)
		}
	}
}

func (p *Pool) sweepLoop(interval time.Duration) {
	defer close(p.done)
	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		select {
		case <-p.stop:
			return
		case now := <-t.C:
			p.closeIdle(now)
		}
	}
}

func (p *Pool) closeIdle(now time.Time) {
	p.mu.Lock()
	defer p.mu.Unlock()
	for _, e := range p.entries {
		if e.leases == 0 && !e.removing && now.Sub(e.lastUsed) >= p.idle {
			p.logClose(p.dropLocked(e), e.id)
		}
	}
}

func (p *Pool) dropLocked(e *entry) error {
	p.lru.Remove(e.elem)
	delete(p.entries, e.id)
	return e.db.close()
}

func (p *Pool) logClose(err error, id string) {
	if err != nil {
		p.log.Warn().Err(err).Str("user_id", id).Msg("close user database")
	}
}

// validateID accepts only the canonical lowercase form, so an id can never
// carry a path separator or name a file other than <uuid>.db.
func validateID(id string) error {
	parsed, err := uuid.Parse(id)
	if err != nil || parsed.String() != id {
		return fmt.Errorf("userdb: %q is not a canonical user id", id)
	}
	return nil
}
