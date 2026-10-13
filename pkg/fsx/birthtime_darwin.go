//go:build darwin

package fsx

import (
	"io/fs"
	"syscall"
	"time"
)

func birthTime(fi fs.FileInfo) (time.Time, bool) {
	st, ok := fi.Sys().(*syscall.Stat_t)
	if !ok {
		return time.Time{}, false
	}
	// A mount that records no birth time reports the epoch, not an error.
	if st.Birthtimespec.Sec == 0 && st.Birthtimespec.Nsec == 0 {
		return time.Time{}, false
	}
	return time.Unix(st.Birthtimespec.Sec, st.Birthtimespec.Nsec), true
}
