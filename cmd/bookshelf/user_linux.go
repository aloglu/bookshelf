package main

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"syscall"
)

// dropPrivileges runs only when Bookshelf starts as root, which is how a
// Docker container starts. It gives the data folder to PUID:PGID (default
// 1000:1000; Unraid uses 99:100), changing only what isn't theirs already,
// and then becomes that user for good, so Bookshelf never runs as root. A
// container started with --user is already that user and skips all this.
func dropPrivileges(dataDir string) error {
	if os.Getuid() != 0 {
		return nil
	}
	uid, err := idFrom("PUID")
	if err != nil {
		return err
	}
	gid, err := idFrom("PGID")
	if err != nil {
		return err
	}
	if uid == 0 || gid == 0 {
		return errors.New("PUID and PGID must not be 0: Bookshelf doesn’t run as root. Use your own user’s numbers (on Unraid, 99 and 100)")
	}
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		return err
	}
	err = filepath.WalkDir(dataDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		if st, ok := info.Sys().(*syscall.Stat_t); ok && (int(st.Uid) != uid || int(st.Gid) != gid) {
			return os.Lchown(path, uid, gid)
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("give the data folder to %d:%d: %w", uid, gid, err)
	}
	// Order matters: groups and group first, while still allowed to change them.
	if err := syscall.Setgroups([]int{gid}); err != nil {
		return err
	}
	if err := syscall.Setgid(gid); err != nil {
		return err
	}
	if err := syscall.Setuid(uid); err != nil {
		return err
	}
	if os.Getuid() == 0 {
		return errors.New("could not stop running as root")
	}
	return nil
}

func idFrom(key string) (int, error) {
	v := os.Getenv(key)
	if v == "" {
		return 1000, nil
	}
	n, err := strconv.Atoi(v)
	if err != nil || n < 0 {
		return 0, fmt.Errorf("%s must be a number, not %q", key, v)
	}
	return n, nil
}
