package library

import (
	"encoding/json"
	"fmt"
	"image"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

func ValidationWarnings(paths Paths, books []Book) ([]string, error) {
	var warnings []string
	raw, err := os.ReadFile(paths.BooksJSON)
	if err != nil {
		return nil, err
	}
	var stored []Book
	if err := json.Unmarshal(raw, &stored); err != nil {
		return nil, err
	}
	referenced := make(map[string]bool)
	for _, book := range stored {
		filename := strings.TrimSpace(book.CoverFile)
		if filename == "" {
			continue
		}
		if filepath.Base(filename) != filename || !strings.EqualFold(filepath.Ext(filename), ".jpg") {
			warnings = append(warnings, fmt.Sprintf("%q references invalid cover filename %q", book.Title, filename))
			continue
		}
		referenced[filename] = true
		coverPath := filepath.Join(paths.CoversDir, filename)
		if !fileExists(coverPath) {
			warnings = append(warnings, fmt.Sprintf("%q references missing cover %q", book.Title, filename))
		} else if problem := coverImageProblem(coverPath); problem != nil {
			warnings = append(warnings, fmt.Sprintf("%q references cover %q that cannot be published: %v", book.Title, filename, problem))
		}
	}
	coverNames, err := regularFileNames(paths.CoversDir)
	if err != nil {
		return nil, err
	}
	for _, filename := range coverNames {
		if strings.EqualFold(filepath.Ext(filename), ".jpg") && !referenced[filename] {
			warnings = append(warnings, fmt.Sprintf("cover %q is not referenced by any book", filename))
		}
	}
	manualNames, err := regularFileNames(paths.ManualCoversDir)
	if err != nil {
		return nil, err
	}
	for _, filename := range manualNames {
		switch {
		case !validManualCoverName(filename):
			warnings = append(warnings, fmt.Sprintf("manual cover %q has an unsupported file type", filename))
		case !manualCoverMatchesAny(filename, books):
			warnings = append(warnings, fmt.Sprintf("manual cover %q does not match any book ID or ISBN", filename))
		}
	}
	seenISBNLess := make(map[string]string)
	for _, book := range books {
		if isbn := CleanISBN(book.ISBN); isbn != "" && !validISBNChecksum(isbn) {
			warnings = append(warnings, fmt.Sprintf("%q has a suspicious ISBN checksum: %s", book.Title, book.ISBN))
		}
		if CleanISBN(book.ISBN) != "" {
			continue
		}
		key := strings.ToLower(strings.TrimSpace(book.Title)) + "\x00" + strings.ToLower(strings.TrimSpace(book.Author))
		if strings.Trim(key, "\x00") == "" {
			continue
		}
		if previous := seenISBNLess[key]; previous != "" {
			warnings = append(warnings, fmt.Sprintf("%q and %q may be duplicate ISBN-less books", previous, book.Title))
		} else {
			seenISBNLess[key] = book.Title
		}
	}
	sort.Strings(warnings)
	return warnings, nil
}

func coverImageProblem(filename string) error {
	input, err := os.Open(filename)
	if err != nil {
		return err
	}
	defer input.Close()
	config, _, err := image.DecodeConfig(input)
	if err != nil {
		return err
	}
	return validateCoverDimensions(config)
}

func validISBNChecksum(isbn string) bool {
	switch len(isbn) {
	case 10:
		total := 0
		for index, character := range isbn {
			value := int(character - '0')
			if index == 9 && (character == 'X' || character == 'x') {
				value = 10
			} else if character < '0' || character > '9' {
				return false
			}
			total += (10 - index) * value
		}
		return total%11 == 0
	case 13:
		total := 0
		for index, character := range isbn {
			if character < '0' || character > '9' {
				return false
			}
			value := int(character - '0')
			if index%2 == 1 {
				value *= 3
			}
			total += value
		}
		return total%10 == 0
	default:
		return false
	}
}
