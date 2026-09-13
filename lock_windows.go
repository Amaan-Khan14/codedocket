//go:build windows

package codedocket

import (
	"os"
	"syscall"
	"unsafe"
)

// LockFileEx dwFlags bit: take an exclusive (writer) lock on the region.
// Omitting LOCKFILE_FAIL_IMMEDIATELY (0x1) makes the call block until the
// range is free.
const lockfileExclusiveLock = 0x00000002

var procLockFileEx = syscall.NewLazyDLL("kernel32.dll").NewProc("LockFileEx")

// lockExclusive takes a blocking exclusive byte-range lock on the first
// byte of the lock file. The OS releases the lock when the handle closes or
// the process exits, so a crashed writer cannot leave the store locked.
// Byte-range locks conflict across handles — including separate handles
// within one process — which is what serializes concurrent Update calls
// (the same property flock gives on unix).
//
// LockFileEx is not exported by the stdlib syscall package, and the module
// is deliberately dependency-free, so we call it through LazyDLL the same
// way golang.org/x/sys does internally.
func lockExclusive(f *os.File) error {
	return lockFileEx(f, lockfileExclusiveLock)
}

func unlockExclusive(f *os.File) error {
	return lockFileEx(f, 0) // zero flags releases our lock on the region
}

func lockFileEx(f *os.File, flags uintptr) error {
	ol := new(syscall.Overlapped)
	r1, _, errno := procLockFileEx.Call(
		f.Fd(),
		flags,
		0, // dwReserved, must be 0
		1, // lock 1 byte at offset 0
		0,
		uintptr(unsafe.Pointer(ol)),
	)
	if r1 == 0 {
		if errno != nil && errno != syscall.Errno(0) {
			return errno
		}
		return syscall.EINVAL
	}
	return nil
}
