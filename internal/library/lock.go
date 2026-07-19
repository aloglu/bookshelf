package library

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"syscall"
	"time"
)

const libraryLockRetry = 25 * time.Millisecond

// WithLibraryLock runs an operation while excluding other Bookshelf readers
// and writers that require a consistent multi-file library snapshot.
func WithLibraryLock(ctx context.Context, paths Paths, operation func() error) error {
	if operation == nil {
		return fmt.Errorf("library lock operation is required")
	}
	unlock, err := acquireLibraryLock(ctx, paths)
	if err != nil {
		return err
	}
	defer unlock()
	return operation()
}

func acquireLibraryLock(ctx context.Context, paths Paths) (func(), error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := os.MkdirAll(paths.Root, 0o755); err != nil {
		return nil, err
	}
	name := filepath.Join(paths.Root, ".bookshelf.lock")
	file, err := os.OpenFile(name, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, err
	}
	for {
		err = syscall.Flock(int(file.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
		if err == nil {
			break
		}
		if !errors.Is(err, syscall.EWOULDBLOCK) && !errors.Is(err, syscall.EAGAIN) {
			file.Close()
			return nil, fmt.Errorf("lock Bookshelf library: %w", err)
		}
		timer := time.NewTimer(libraryLockRetry)
		select {
		case <-ctx.Done():
			if !timer.Stop() {
				<-timer.C
			}
			file.Close()
			return nil, ctx.Err()
		case <-timer.C:
		}
	}

	var once sync.Once
	return func() {
		once.Do(func() {
			_ = syscall.Flock(int(file.Fd()), syscall.LOCK_UN)
			_ = file.Close()
		})
	}, nil
}
