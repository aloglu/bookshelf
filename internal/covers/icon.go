package covers

import (
	"errors"
	"fmt"
	"image"
	"image/png"
	"io"
	"os"
	"path/filepath"
	"regexp"

	"golang.org/x/image/draw"
)

// The library's own icon, if the owner chooses one: any picture, cropped to a
// square at its middle, written as PNG (what browsers take for a tab or a
// home screen) at the sizes they ask for, under a new name each time so no
// browser keeps showing the old one.

// IconSizes are the icon's sizes: a tab, an iPhone's home screen, Android's.
var IconSizes = []int{32, 180, 192, 512}

var iconName = regexp.MustCompile(`^[0-9a-f]{16}$`)

// IconFile is the file for the icon called name at a size.
func IconFile(name string, size int) string { return fmt.Sprintf("%s-%d.png", name, size) }

// ValidIconName reports whether name could be an icon SaveIcon wrote.
func ValidIconName(name string) bool { return iconName.MatchString(name) }

// SaveIcon reads a picture and writes the icon's files into dir, returning
// its name. Files are written whole or not at all.
func SaveIcon(dir string, src io.Reader) (string, error) {
	raw, err := io.ReadAll(io.LimitReader(src, MaxUpload+1))
	if err != nil {
		return "", err
	}
	if len(raw) > MaxUpload {
		return "", fmt.Errorf("the picture is larger than %d MB", MaxUpload>>20)
	}
	img, err := decode(raw)
	if err != nil {
		return "", err
	}
	sq := middleSquare(img)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	name := suffix() + suffix()
	var written []string
	for _, size := range IconSizes {
		dst := image.NewRGBA(image.Rect(0, 0, size, size))
		draw.CatmullRom.Scale(dst, dst.Bounds(), img, sq, draw.Src, nil)
		path := filepath.Join(dir, IconFile(name, size))
		if err := writePNG(path, dst); err != nil {
			for _, p := range written {
				os.Remove(p)
			}
			return "", err
		}
		written = append(written, path)
	}
	return name, nil
}

// RemoveIcon deletes an icon's files. Missing files are not an error.
func RemoveIcon(dir, name string) error {
	if !ValidIconName(name) {
		return nil
	}
	var errs []error
	for _, size := range IconSizes {
		if err := os.Remove(filepath.Join(dir, IconFile(name, size))); err != nil && !errors.Is(err, os.ErrNotExist) {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

// middleSquare is the largest square in the middle of the picture.
func middleSquare(img image.Image) image.Rectangle {
	b := img.Bounds()
	side := min(b.Dx(), b.Dy())
	x, y := b.Min.X+(b.Dx()-side)/2, b.Min.Y+(b.Dy()-side)/2
	return image.Rect(x, y, x+side, y+side)
}

func writePNG(path string, img image.Image) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), ".icon-*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if err := png.Encode(tmp, img); err != nil {
		tmp.Close()
		return fmt.Errorf("write icon: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Chmod(tmp.Name(), 0o644); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), path)
}
