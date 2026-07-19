package library

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func recoverDataIfNeeded(paths Paths) error {
	previous := paths.DataDir + ".previous"
	if _, err := os.Stat(previous); errors.Is(err, os.ErrNotExist) {
		return nil
	} else if err != nil {
		return err
	}
	unlock, err := acquireLibraryLock(context.Background(), paths)
	if err != nil {
		return err
	}
	defer unlock()
	return recoverDataDirectory(paths)
}

func recoverDataDirectory(paths Paths) error {
	previousDir := paths.DataDir + ".previous"
	if _, err := os.Stat(previousDir); errors.Is(err, os.ErrNotExist) {
		return nil
	} else if err != nil {
		return err
	}

	if _, err := os.Stat(paths.DataDir); err == nil {
		if err := validateRecoveryData(paths); err != nil {
			return fmt.Errorf(
				"cannot recover Bookshelf data automatically: both %s and %s exist, but current data is incomplete: %w",
				paths.DataDir, previousDir, err,
			)
		}
		if err := os.RemoveAll(previousDir); err != nil {
			return fmt.Errorf("remove completed Bookshelf recovery backup %s: %w", previousDir, err)
		}
		return removeStaleImportStages(paths.Root)
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}

	previousPaths := withDataDirectory(paths, previousDir)
	if err := validateRecoveryData(previousPaths); err != nil {
		return fmt.Errorf("cannot recover invalid Bookshelf data from %s: %w", previousDir, err)
	}
	if err := os.Rename(previousDir, paths.DataDir); err != nil {
		return fmt.Errorf("restore Bookshelf data from %s: %w", previousDir, err)
	}
	return removeStaleImportStages(paths.Root)
}

func validateRecoveryData(paths Paths) error {
	if !OwnsRoot(paths) {
		return fmt.Errorf("ownership marker is missing")
	}
	if _, err := Load(paths); err != nil {
		return err
	}
	return nil
}

func withDataDirectory(paths Paths, dataDir string) Paths {
	paths.DataDir = dataDir
	paths.RootMarker = filepath.Join(dataDir, ".bookshelf-root")
	paths.BooksJSON = filepath.Join(dataDir, "books.json")
	paths.ConfigJSON = filepath.Join(dataDir, "settings.json")
	paths.CoverReportJSON = filepath.Join(dataDir, "cover-report.json")
	paths.CoversDir = filepath.Join(dataDir, "covers")
	paths.ManualCoversDir = filepath.Join(dataDir, "manual-covers")
	return paths
}

func removeStaleImportStages(root string) error {
	entries, err := os.ReadDir(root)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), ".bookshelf-import-") {
			if err := os.RemoveAll(filepath.Join(root, entry.Name())); err != nil {
				return err
			}
		}
	}
	return nil
}
