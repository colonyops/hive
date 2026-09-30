// Package procstats samples what this install is costing the machine: the
// desktop process, the processes it parents (a terminal's shell, an agent), and
// the Go runtime's own view of itself.
//
// The process numbers and the Go numbers answer different questions and are
// kept apart deliberately: RSS is what the OS charges for, while heap-in-use is
// what this program asked for, and on a cgo-heavy shell the gap between them is
// the native side — the part the Go runtime cannot see.
//
// What the walk cannot reach: on macOS the WebKit processes that render the UI
// are XPC services launchd parents, not children of this process, and their
// only link back to the app is a responsible-pid the public API does not
// expose. They are excluded rather than guessed at — a WebContent process on
// the machine may well belong to another app.
package procstats

import (
	"cmp"
	"context"
	"runtime"
	"slices"
	"sync"
	"time"

	"github.com/shirou/gopsutil/v4/process"
)

// maxProcesses bounds a walk of the process tree. A terminal-heavy session can
// parent a lot of shells; past this the panel is measuring noise, and the
// sample has a per-process syscall cost the caller repeats every few seconds.
const maxProcesses = 64

// Process is one sampled OS process.
type Process struct {
	PID  int32  `json:"pid"`
	Name string `json:"name"`
	// RSSBytes is resident set size — the memory the OS is actually holding
	// for this process, which is the number Activity Monitor shows.
	RSSBytes uint64 `json:"rssBytes"`
	// CPUPercent is the share of one core used since the previous sample, so
	// it can exceed 100 on a process using several cores. The first sample for
	// a process reports its average since it started, there being nothing to
	// difference against yet.
	CPUPercent float64 `json:"cpuPercent"`
	Threads    int32   `json:"threads"`
}

// GoRuntime is the Go runtime's own accounting for this process.
type GoRuntime struct {
	Goroutines     int     `json:"goroutines"`
	GOMAXPROCS     int     `json:"gomaxprocs"`
	NumCPU         int     `json:"numCpu"`
	HeapAllocBytes uint64  `json:"heapAllocBytes"`
	HeapSysBytes   uint64  `json:"heapSysBytes"`
	HeapObjects    uint64  `json:"heapObjects"`
	StackSysBytes  uint64  `json:"stackSysBytes"`
	TotalSysBytes  uint64  `json:"totalSysBytes"`
	NextGCBytes    uint64  `json:"nextGcBytes"`
	GCCount        uint32  `json:"gcCount"`
	LastGCUnixMs   int64   `json:"lastGcUnixMs"`
	LastPauseMs    float64 `json:"lastPauseMs"`
	TotalPauseMs   float64 `json:"totalPauseMs"`
}

// Stats is one sample: the app process, its descendants, and the Go runtime.
type Stats struct {
	SampledAt time.Time `json:"sampledAt"`
	UptimeMs  int64     `json:"uptimeMs"`
	Process   Process   `json:"process"`
	// Children is the process tree below this one, biggest first — the part a
	// Go-only view misses entirely.
	Children []Process `json:"children"`
	// ChildrenTruncated reports that the tree was larger than the walk's cap,
	// so the totals below are a floor rather than the whole picture.
	ChildrenTruncated bool      `json:"childrenTruncated"`
	Go                GoRuntime `json:"go"`
}

// TotalRSSBytes is the whole tree's resident memory, this process included.
func (s Stats) TotalRSSBytes() uint64 {
	total := s.Process.RSSBytes
	for _, c := range s.Children {
		total += c.RSSBytes
	}
	return total
}

// TotalCPUPercent is the whole tree's CPU share, this process included.
func (s Stats) TotalCPUPercent() float64 {
	total := s.Process.CPUPercent
	for _, c := range s.Children {
		total += c.CPUPercent
	}
	return total
}

// cpuMark is the previous sample's cumulative CPU time for one process, which
// is what makes the next sample a rate rather than a lifetime average.
type cpuMark struct {
	seconds float64
	at      time.Time
}

// Sampler holds the previous sample so CPU can be reported as a rate. It is
// safe for concurrent use; a UI polling it and a report assembling one at the
// same time must not race on the mark table.
type Sampler struct {
	pid     int32
	started time.Time
	now     func() time.Time

	mu    sync.Mutex
	marks map[int32]cpuMark
}

// New returns a sampler for the current process, started now.
func New(pid int32) *Sampler {
	return &Sampler{pid: pid, started: time.Now(), now: time.Now, marks: map[int32]cpuMark{}}
}

// Sample reads the process tree and the Go runtime. It returns an error only
// when this process cannot be read at all; a child that exits mid-walk is
// dropped from the sample rather than failing it, which is the common case on
// a machine doing anything.
func (s *Sampler) Sample(ctx context.Context) (Stats, error) {
	self, err := process.NewProcessWithContext(ctx, s.pid)
	if err != nil {
		return Stats{}, err
	}

	now := s.now()
	stats := Stats{
		SampledAt: now,
		UptimeMs:  now.Sub(s.started).Milliseconds(),
		Process:   s.read(ctx, self, now),
		Go:        readGoRuntime(),
	}

	seen := map[int32]bool{s.pid: true}
	queue := []*process.Process{self}
	for len(queue) > 0 && len(stats.Children) < maxProcesses {
		children, err := queue[0].ChildrenWithContext(ctx)
		queue = queue[1:]
		if err != nil {
			continue
		}
		for _, child := range children {
			if seen[child.Pid] {
				continue
			}
			seen[child.Pid] = true
			if len(stats.Children) >= maxProcesses {
				stats.ChildrenTruncated = true
				break
			}
			stats.Children = append(stats.Children, s.read(ctx, child, now))
			queue = append(queue, child)
		}
	}

	slices.SortFunc(stats.Children, func(a, b Process) int { return cmp.Compare(b.RSSBytes, a.RSSBytes) })
	s.forget(seen)
	return stats, nil
}

func (s *Sampler) read(ctx context.Context, p *process.Process, now time.Time) Process {
	out := Process{PID: p.Pid}
	if name, err := p.NameWithContext(ctx); err == nil {
		out.Name = name
	}
	if mem, err := p.MemoryInfoWithContext(ctx); err == nil && mem != nil {
		out.RSSBytes = mem.RSS
	}
	if threads, err := p.NumThreadsWithContext(ctx); err == nil {
		out.Threads = threads
	}
	out.CPUPercent = s.cpuPercent(ctx, p, now)
	return out
}

// cpuPercent differences the process's cumulative CPU time against the previous
// sample. Without a previous one it falls back to the average since the process
// started, so the first paint of the panel is not a row of zeros.
func (s *Sampler) cpuPercent(ctx context.Context, p *process.Process, now time.Time) float64 {
	times, err := p.TimesWithContext(ctx)
	if err != nil || times == nil {
		return 0
	}
	used := times.User + times.System

	s.mu.Lock()
	previous, ok := s.marks[p.Pid]
	s.marks[p.Pid] = cpuMark{seconds: used, at: now}
	s.mu.Unlock()

	if ok {
		elapsed := now.Sub(previous.at).Seconds()
		if elapsed <= 0 {
			return 0
		}
		return percent(used-previous.seconds, elapsed)
	}

	createdMs, err := p.CreateTimeWithContext(ctx)
	if err != nil || createdMs <= 0 {
		return 0
	}
	return percent(used, now.Sub(time.UnixMilli(createdMs)).Seconds())
}

func percent(cpuSeconds, elapsedSeconds float64) float64 {
	if elapsedSeconds <= 0 || cpuSeconds <= 0 {
		return 0
	}
	return cpuSeconds / elapsedSeconds * 100
}

// forget drops marks for processes that were not in this walk, so a session
// that churns short-lived shells does not grow the table forever.
func (s *Sampler) forget(seen map[int32]bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for pid := range s.marks {
		if !seen[pid] {
			delete(s.marks, pid)
		}
	}
}

func readGoRuntime() GoRuntime {
	var mem runtime.MemStats
	runtime.ReadMemStats(&mem)

	out := GoRuntime{
		Goroutines:     runtime.NumGoroutine(),
		GOMAXPROCS:     runtime.GOMAXPROCS(0),
		NumCPU:         runtime.NumCPU(),
		HeapAllocBytes: mem.HeapAlloc,
		HeapSysBytes:   mem.HeapSys,
		HeapObjects:    mem.HeapObjects,
		StackSysBytes:  mem.StackSys,
		TotalSysBytes:  mem.Sys,
		NextGCBytes:    mem.NextGC,
		GCCount:        mem.NumGC,
		TotalPauseMs:   float64(mem.PauseTotalNs) / 1e6,
	}
	if mem.LastGC > 0 {
		out.LastGCUnixMs = int64(mem.LastGC / 1e6)
		out.LastPauseMs = float64(mem.PauseNs[(mem.NumGC+255)%256]) / 1e6
	}
	return out
}
