package library

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestWithLibraryLockWaitIsCancellable(t *testing.T) {
	paths := fixture(t)
	locked := make(chan struct{})
	release := make(chan struct{})
	holderDone := make(chan error, 1)
	go func() {
		holderDone <- WithLibraryLock(context.Background(), paths, func() error {
			close(locked)
			<-release
			return nil
		})
	}()
	<-locked

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	err := WithLibraryLock(ctx, paths, func() error {
		t.Fatal("cancelled lock operation ran")
		return nil
	})
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("lock error = %v, want context deadline exceeded", err)
	}

	close(release)
	if err := <-holderDone; err != nil {
		t.Fatal(err)
	}
}

func TestRemoveAndCoverCommitLockWaitsAreCancellable(t *testing.T) {
	for _, test := range []struct {
		name string
		run  func(context.Context, Paths) error
	}{
		{
			name: "remove",
			run: func(ctx context.Context, paths Paths) error {
				books, err := Load(paths)
				if err != nil {
					return err
				}
				_, err = Remove(ctx, paths, []string{books[0].ID}, false)
				return err
			},
		},
		{
			name: "cover commit",
			run: func(ctx context.Context, paths Paths) error {
				books, err := Load(paths)
				if err != nil {
					return err
				}
				session, err := NewCoverFetchSession(paths, books, false)
				if err != nil {
					return err
				}
				defer session.Discard()
				_, err = session.Commit(ctx)
				return err
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			paths := fixture(t)
			if err := Save(paths, []Book{Normalize(Book{Title: "Dune"})}); err != nil {
				t.Fatal(err)
			}
			locked := make(chan struct{})
			release := make(chan struct{})
			holderDone := make(chan error, 1)
			go func() {
				holderDone <- WithLibraryLock(context.Background(), paths, func() error {
					close(locked)
					<-release
					return nil
				})
			}()
			<-locked

			ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
			err := test.run(ctx, paths)
			cancel()
			if !errors.Is(err, context.DeadlineExceeded) {
				t.Fatalf("operation error = %v", err)
			}
			close(release)
			if err := <-holderDone; err != nil {
				t.Fatal(err)
			}
		})
	}
}
