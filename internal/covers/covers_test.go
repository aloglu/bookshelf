package covers

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// A cover that is dark red at its left edge and white elsewhere.
func testCover(t *testing.T) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 600, 900))
	for y := range 900 {
		for x := range 600 {
			c := color.RGBA{250, 250, 250, 255}
			if x < 80 {
				c = color.RGBA{120, 20, 30, 255}
			}
			img.Set(x, y, c)
		}
	}
	var buf bytes.Buffer
	png.Encode(&buf, img)
	return buf.Bytes()
}

func TestSave(t *testing.T) {
	dir := t.TempDir()
	res, err := Save(dir, "abc123", bytes.NewReader(testCover(t)))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(res.Name, "abc123-") || !strings.HasSuffix(res.Name, ".webp") {
		t.Fatalf("name %q", res.Name)
	}
	for _, n := range []string{res.Name, ThumbName(res.Name)} {
		f, err := os.Open(filepath.Join(dir, n))
		if err != nil {
			t.Fatal(err)
		}
		cfg, format, err := image.DecodeConfig(f)
		f.Close()
		if err != nil || format != "webp" {
			t.Fatalf("%s: %v %s", n, err, format)
		}
		if n == res.Name && cfg.Width != 600 || n != res.Name && cfg.Width != ThumbWidth {
			t.Fatalf("%s is %d wide", n, cfg.Width)
		}
	}
	if res.Spine != "#78141e" || res.SpineText != "#f7f1e6" {
		t.Fatalf("spine %s / %s", res.Spine, res.SpineText)
	}
	// No temporary files are left behind.
	entries, _ := os.ReadDir(dir)
	if len(entries) != 2 {
		t.Fatalf("%d files in the covers folder, want 2", len(entries))
	}
	if err := Remove(dir, res.Name); err != nil {
		t.Fatal(err)
	}
	if entries, _ := os.ReadDir(dir); len(entries) != 0 {
		t.Fatalf("files left after Remove: %d", len(entries))
	}
}

func TestRejectsNonImages(t *testing.T) {
	if _, err := Save(t.TempDir(), "x", strings.NewReader("not an image")); err != ErrNotImage {
		t.Fatalf("got %v", err)
	}
}

func TestTextFor(t *testing.T) {
	if TextFor("#f2e6c8") != "#221a14" || TextFor("#1b2a3c") != "#f7f1e6" {
		t.Fatal("lettering colors")
	}
}
