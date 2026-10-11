//go:build !linux

package main

// dropPrivileges only matters in a Docker container, which runs Linux.
func dropPrivileges(string) error { return nil }
