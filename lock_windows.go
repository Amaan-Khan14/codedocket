//go:build windows

package codedocket

import (
	"os"
	"syscall"
)

// lockExclusive takes a blocking exclusive byte-range lock (LockFileEx) on
// the first byte of the lock file. Without LOCKFILE_FAIL_IMMEDIATELY the call
// blocks until the range is free; the OS releases it when the handle closes
// or the process exits, so a crashed writer cannot leave the store locked.
func lockExclusive(f *os.File) error {
	return syscall.LockFileEx(
		syscall.Handle(f.Fd()),
		syscall.LOCKFILE_EXCLUSIVE_LOCK,
		0, 1, 0, new(syscall.Overlapped),
	)
}

func unlockExclusive(f *os.File) error {
	return syscall.LockFileEx(
		syscall.Handle(f.Fd()),
		0, // no flags: releases our exclusive lock on the range
		0, 1, 0, new(syscall.Overlapped),
	)
}
