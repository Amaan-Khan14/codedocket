//go:build !windows

package codedocket

import (
	"os"
	"syscall"
)

// lockExclusive takes a blocking exclusive flock on the lock file. The
// kernel releases it when the file descriptor closes or the process exits,
// so a crashed writer cannot leave the store permanently locked.
func lockExclusive(f *os.File) error {
	return syscall.Flock(int(f.Fd()), syscall.LOCK_EX)
}

func unlockExclusive(f *os.File) error {
	return syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
}
