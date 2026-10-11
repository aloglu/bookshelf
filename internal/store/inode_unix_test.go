//go:build unix

package store

import (
	"os"
	"syscall"
)

func inode(info os.FileInfo) uint64 { return info.Sys().(*syscall.Stat_t).Ino }
