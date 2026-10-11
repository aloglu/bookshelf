// Package covers turns an uploaded or fetched cover image into the files the
// site shows, and works out a book's spine colors from it.
package covers

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"

	gowebp "github.com/linzeyan/webp-go"
	_ "golang.org/x/image/bmp"
	"golang.org/x/image/draw"
	_ "golang.org/x/image/webp"
)

const (
	// DetailWidth suits the open book's cover plate on a 2× screen.
	DetailWidth = 900
	// ThumbWidth suits the small covers (the header, the stats page, a phone's title page).
	ThumbWidth = 360
	// MaxUpload is the largest cover file accepted.
	MaxUpload = 25 << 20

	maxSide   = 20_000
	maxPixels = 50_000_000
	quality   = 82
	method    = 4
)

// ErrNotImage means the file isn't an image Bookshelf can read.
var ErrNotImage = errors.New("this file isn’t an image Bookshelf can read (JPEG, PNG, WebP, GIF or BMP)")

// Result is a processed cover.
type Result struct {
	Name      string // file name of the detail image; the thumbnail is ThumbName(Name)
	Spine     string // spine color, #rrggbb
	SpineText string // lettering color for that spine
}

// ThumbName is the thumbnail's file name for a cover.
func ThumbName(name string) string { return strings.TrimSuffix(name, ".webp") + "-thumb.webp" }

// Save reads an image, writes its detail image and thumbnail into dir under a
// new name (so a replaced cover never shows from a browser's cache), and
// works out the spine colors. Files are written whole or not at all.
func Save(dir, bookID string, src io.Reader) (Result, error) {
	raw, err := io.ReadAll(io.LimitReader(src, MaxUpload+1))
	if err != nil {
		return Result{}, err
	}
	if len(raw) > MaxUpload {
		return Result{}, fmt.Errorf("the cover is larger than %d MB", MaxUpload>>20)
	}
	img, err := decode(raw)
	if err != nil {
		return Result{}, err
	}
	spine, text := SpineColors(img)

	if err := os.MkdirAll(dir, 0o755); err != nil {
		return Result{}, err
	}
	name := safe(bookID) + "-" + suffix() + ".webp"
	if err := writeWebP(filepath.Join(dir, name), resize(img, DetailWidth)); err != nil {
		return Result{}, err
	}
	if err := writeWebP(filepath.Join(dir, ThumbName(name)), resize(img, ThumbWidth)); err != nil {
		os.Remove(filepath.Join(dir, name))
		return Result{}, err
	}
	return Result{Name: name, Spine: spine, SpineText: text}, nil
}

// Remove deletes a cover and its thumbnail. Missing files are not an error.
func Remove(dir, name string) error {
	if name == "" || name != filepath.Base(name) {
		return nil
	}
	var errs []error
	for _, n := range []string{name, ThumbName(name)} {
		if err := os.Remove(filepath.Join(dir, n)); err != nil && !os.IsNotExist(err) {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

func decode(raw []byte) (image.Image, error) {
	cfg, _, err := image.DecodeConfig(bytes.NewReader(raw))
	if err != nil {
		return nil, ErrNotImage
	}
	if cfg.Width <= 0 || cfg.Height <= 0 || cfg.Width > maxSide || cfg.Height > maxSide || int64(cfg.Width)*int64(cfg.Height) > maxPixels {
		return nil, fmt.Errorf("the image is %d×%d; covers can be up to %d pixels on a side", cfg.Width, cfg.Height, maxSide)
	}
	img, _, err := image.Decode(bytes.NewReader(raw))
	if err != nil {
		return nil, ErrNotImage
	}
	return img, nil
}

func resize(src image.Image, width int) image.Image {
	b := src.Bounds()
	w := min(b.Dx(), width)
	h := max(1, b.Dy()*w/b.Dx())
	dst := image.NewRGBA(image.Rect(0, 0, w, h))
	draw.CatmullRom.Scale(dst, dst.Bounds(), src, b, draw.Over, nil)
	return dst
}

func writeWebP(path string, img image.Image) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), ".cover-*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if err := gowebp.Encode(tmp, img, &gowebp.Options{Lossy: true, Quality: quality, Method: method}); err != nil {
		tmp.Close()
		return fmt.Errorf("encode cover: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), path)
}

func suffix() string {
	b := make([]byte, 4)
	rand.Read(b)
	return hex.EncodeToString(b)
}

func safe(id string) string {
	var b strings.Builder
	for _, r := range id {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' {
			b.WriteRune(r)
		}
	}
	if b.Len() == 0 {
		return "book"
	}
	return b.String()
}

// SpineColors works out a spine's color from its cover: the cover scaled to
// 48×72, then the median of a strip just inside its spine-side (left) edge,
// skipping the outermost column and the corners, as a printed jacket's edge
// is usually what wraps round onto the spine. The lettering is dark on light
// spines and light on dark ones.
func SpineColors(img image.Image) (spine, text string) {
	small := image.NewRGBA(image.Rect(0, 0, 48, 72))
	draw.ApproxBiLinear.Scale(small, small.Bounds(), img, img.Bounds(), draw.Src, nil)
	var rs, gs, bs []uint8
	for y := 3; y < 69; y++ {
		for x := 1; x < 5; x++ {
			c := small.RGBAAt(x, y)
			rs, gs, bs = append(rs, c.R), append(gs, c.G), append(bs, c.B)
		}
	}
	median := func(v []uint8) uint8 { slices.Sort(v); return v[len(v)/2] }
	r, g, b := median(rs), median(gs), median(bs)
	spine = fmt.Sprintf("#%02x%02x%02x", r, g, b)
	return spine, TextFor(spine)
}

// TextFor is the lettering color for a spine color.
func TextFor(hex string) string {
	var r, g, b int
	if _, err := fmt.Sscanf(hex, "#%02x%02x%02x", &r, &g, &b); err != nil {
		return "#f7f1e6"
	}
	if (0.2126*float64(r)+0.7152*float64(g)+0.0722*float64(b))/255 > 0.58 {
		return "#221a14"
	}
	return "#f7f1e6"
}
