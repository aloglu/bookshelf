package library

import (
	"context"
	"fmt"
	"image"
	"image/jpeg"
	_ "image/png"
	"os"
	"path/filepath"
	"strings"

	_ "golang.org/x/image/bmp"
	_ "golang.org/x/image/webp"
)

type BuildOptions struct {
	RecomputeColors bool
	ProcessOnly     map[string]bool
}

type BuildStats struct {
	Books     int
	Processed int
	Manuals   int
	Colored   int
	Missing   int
}

func Build(ctx context.Context, paths Paths, options BuildOptions) (BuildStats, error) {
	var stats BuildStats
	if err := Ensure(paths); err != nil {
		return stats, err
	}

	books, err := Load(paths)
	if err != nil {
		return stats, err
	}
	config, err := LoadConfig(paths)
	if err != nil {
		return stats, err
	}
	if problems := Validate(books); len(problems) > 0 {
		return stats, validationError(problems)
	}
	stats.Books = len(books)
	var renameRollbacks []func()
	committed := false
	defer func() {
		if committed {
			return
		}
		for index := len(renameRollbacks) - 1; index >= 0; index-- {
			renameRollbacks[index]()
		}
	}()

	for i := range books {
		book := &books[i]
		if len(options.ProcessOnly) > 0 && !options.ProcessOnly[book.Key()] {
			continue
		}
		stats.Processed++

		previous := *book
		rollbackRename, err := renameCoverForBook(paths, previous, book)
		if err != nil {
			return stats, err
		}
		renameRollbacks = append(renameRollbacks, rollbackRename)
		filename := coverFilename(*book)
		if filename == "" {
			filename = preferredCoverFilename(*book)
		}
		destination := filepath.Join(paths.CoversDir, filename)
		manual := findManualCover(paths, *book)
		if manual != "" {
			if err := transcodeJPEG(manual, destination); err != nil {
				return stats, fmt.Errorf("process manual cover for %q: %w", book.Title, err)
			}
			stats.Manuals++
		}

		if fileExists(destination) {
			book.CoverFile = filename
			book.Cover = filepath.ToSlash(filepath.Join("data", "covers", filename))
			if options.RecomputeColors || book.SpineColor == "" || book.SpineTextColor == "" {
				background, foreground, colorErr := extractPalette(destination)
				if colorErr == nil {
					book.SpineColor = background
					book.SpineTextColor = foreground
					stats.Colored++
				}
			}
		} else {
			book.Cover = ""
			book.SpineColor = ""
			book.SpineTextColor = ""
			stats.Missing++
		}
	}

	if err := Save(paths, books); err != nil {
		return stats, err
	}
	committed = true
	if err := SaveConfig(paths, config); err != nil {
		return stats, err
	}
	if err := SaveGenerated(paths, books); err != nil {
		return stats, err
	}
	return stats, nil
}

func ApplyManualCovers(ctx context.Context, paths Paths, ids []string, recompute bool) (BuildStats, error) {
	books, err := Load(paths)
	if err != nil {
		return BuildStats{}, err
	}
	only := make(map[string]bool)
	for _, id := range ids {
		index := FindIndex(books, id)
		if index < 0 {
			return BuildStats{}, fmt.Errorf("no book found for %q", id)
		}
		only[books[index].Key()] = true
	}
	return Build(ctx, paths, BuildOptions{
		RecomputeColors: recompute,
		ProcessOnly:     only,
	})
}

func coverFilename(book Book) string {
	filename := strings.TrimSpace(book.CoverFile)
	if filename == "" || filepath.Base(filename) != filename || !strings.EqualFold(filepath.Ext(filename), ".jpg") {
		return ""
	}
	return filename
}

func preferredCoverFilename(book Book) string {
	token := formattedISBNToken(book.ISBN)
	if token == "" {
		token = safeToken(book.ID)
	}
	if token == "" {
		return ""
	}
	return token + ".jpg"
}

func formattedISBNToken(value string) string {
	var token strings.Builder
	for _, character := range strings.TrimSpace(value) {
		switch {
		case character >= '0' && character <= '9':
			token.WriteRune(character)
		case character == 'x' || character == 'X' || character == '-':
			token.WriteRune(character)
		}
	}
	return token.String()
}

func safeToken(value string) string {
	value = strings.TrimSpace(value)
	value = strings.ReplaceAll(value, string(filepath.Separator), "-")
	value = strings.ReplaceAll(value, "/", "-")
	value = strings.ReplaceAll(value, "\\", "-")
	value = strings.Trim(value, ".- ")
	if value == "" {
		return Slugify(value)
	}
	return value
}

func findManualCover(paths Paths, book Book) string {
	candidates := []string{formattedISBNToken(book.ISBN), CleanISBN(book.ISBN), safeToken(book.ID)}
	seen := make(map[string]bool, len(candidates))
	extensions := []string{".jpg", ".jpeg", ".png", ".webp", ".bmp"}
	for _, candidate := range candidates {
		if candidate == "" || seen[candidate] {
			continue
		}
		seen[candidate] = true
		for _, extension := range extensions {
			name := filepath.Join(paths.ManualCoversDir, candidate+extension)
			if fileExists(name) {
				return name
			}
		}
	}
	return ""
}

func transcodeJPEG(source, destination string) error {
	input, err := os.Open(source)
	if err != nil {
		return err
	}
	defer input.Close()
	img, _, err := image.Decode(input)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(destination), 0o755); err != nil {
		return err
	}
	temp, err := os.CreateTemp(filepath.Dir(destination), ".cover-*.jpg")
	if err != nil {
		return err
	}
	tempName := temp.Name()
	defer os.Remove(tempName)
	if err := jpeg.Encode(temp, img, &jpeg.Options{Quality: 90}); err != nil {
		temp.Close()
		return err
	}
	if err := temp.Close(); err != nil {
		return err
	}
	return os.Rename(tempName, destination)
}

func extractPalette(name string) (string, string, error) {
	file, err := os.Open(name)
	if err != nil {
		return "", "", err
	}
	defer file.Close()
	img, _, err := image.Decode(file)
	if err != nil {
		return "", "", err
	}
	bounds := img.Bounds()
	stepX := max(1, bounds.Dx()/64)
	stepY := max(1, bounds.Dy()/64)
	var red, green, blue, count uint64
	for y := bounds.Min.Y; y < bounds.Max.Y; y += stepY {
		for x := bounds.Min.X; x < bounds.Max.X; x += stepX {
			r, g, b, a := img.At(x, y).RGBA()
			if a == 0 {
				continue
			}
			red += uint64(r >> 8)
			green += uint64(g >> 8)
			blue += uint64(b >> 8)
			count++
		}
	}
	if count == 0 {
		return "", "", fmt.Errorf("image has no visible pixels")
	}
	r := uint8(red / count)
	g := uint8(green / count)
	b := uint8(blue / count)
	luminance := (0.2126*float64(r) + 0.7152*float64(g) + 0.0722*float64(b)) / 255
	foreground := "#fdfdfd"
	if luminance > 0.55 {
		foreground = "#1c1c22"
	}
	return fmt.Sprintf("#%02X%02X%02X", r, g, b), foreground, nil
}

func validationError(problems []error) error {
	var text strings.Builder
	text.WriteString("validation failed:")
	for _, problem := range problems {
		text.WriteString("\n- ")
		text.WriteString(problem.Error())
	}
	return fmt.Errorf("%s", text.String())
}
