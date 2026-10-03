package plugins

import "context"

// WorkerPool limits concurrent subprocess execution across all plugins.
type WorkerPool struct {
	sem chan struct{}
}

// NewWorkerPool creates a new worker pool with the given size.
func NewWorkerPool(size int) *WorkerPool {
	if size <= 0 {
		size = 5
	}
	return &WorkerPool{
		sem: make(chan struct{}, size),
	}
}

// Acquire blocks until a worker slot is available.
func (p *WorkerPool) Acquire() {
	p.sem <- struct{}{}
}

// Release returns a worker slot to the pool.
func (p *WorkerPool) Release() {
	<-p.sem
}

// Run executes fn with pool semaphore held.
func (p *WorkerPool) Run(fn func()) {
	p.Acquire()
	defer p.Release()
	fn()
}

// RunContext executes fn with pool semaphore held, respecting context cancellation.
// Returns ctx.Err() if context is cancelled while waiting to acquire.
func (p *WorkerPool) RunContext(ctx context.Context, fn func()) error {
	select {
	case p.sem <- struct{}{}:
		defer p.Release()
		fn()
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
