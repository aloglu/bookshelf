package library

import (
	"bytes"
	"context"
	"errors"
	"image"
	"image/color"
	"image/jpeg"
	"os"
	"path/filepath"
	"strings"
	"testing"

	gowebp "github.com/linzeyan/webp-go"
)

func TestGenerateWebCoverVariantsPreserveRatioAndUseWebP(t *testing.T) {
	directory := t.TempDir()
	source := filepath.Join(directory, "cover.jpg")
	output, err := os.Create(source)
	if err != nil {
		t.Fatal(err)
	}
	original := image.NewRGBA(image.Rect(0, 0, 800, 1200))
	for y := 0; y < original.Bounds().Dy(); y++ {
		for x := 0; x < original.Bounds().Dx(); x++ {
			original.Set(x, y, color.RGBA{R: 80, G: 40, B: 120, A: 255})
		}
	}
	if err := jpeg.Encode(output, original, &jpeg.Options{Quality: 95}); err != nil {
		t.Fatal(err)
	}
	if err := output.Close(); err != nil {
		t.Fatal(err)
	}

	thumbnail := filepath.Join(directory, "thumbnail.webp")
	detail := filepath.Join(directory, "detail.webp")
	if err := generateWebCoverVariants(source, thumbnail, detail); err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		filename string
		width    int
		height   int
	}{
		{filename: thumbnail, width: websiteThumbnailWidth, height: 540},
		{filename: detail, width: websiteDetailWidth, height: 720},
	} {
		input, err := os.Open(test.filename)
		if err != nil {
			t.Fatal(err)
		}
		config, err := gowebp.DecodeConfig(input)
		input.Close()
		if err != nil {
			t.Fatal(err)
		}
		if config.Width != test.width || config.Height != test.height {
			t.Fatalf("%s dimensions = %dx%d", filepath.Base(test.filename), config.Width, config.Height)
		}
	}
}

func TestGeneratedWebCoverFilenameKeepsReadableISBN(t *testing.T) {
	if got := generatedWebCoverFilename("978-0-441-17271-9.jpg"); got != "978-0-441-17271-9.webp" {
		t.Fatalf("generated filename = %q", got)
	}
}

func TestResizeWebCoverDoesNotUpscale(t *testing.T) {
	source := image.NewRGBA(image.Rect(0, 0, 200, 300))
	resized := resizeWebCover(source, websiteDetailWidth)
	if resized.Bounds().Dx() != 200 || resized.Bounds().Dy() != 300 {
		t.Fatalf("resized dimensions = %dx%d", resized.Bounds().Dx(), resized.Bounds().Dy())
	}
}

func TestSmallWebCoverUsesIdenticalThumbnailAndDetailVariants(t *testing.T) {
	directory := t.TempDir()
	source := filepath.Join(directory, "cover.jpg")
	writeTestJPEG(t, source)
	thumbnail := filepath.Join(directory, "thumbnail.webp")
	detail := filepath.Join(directory, "detail.webp")
	if err := generateWebCoverVariants(source, thumbnail, detail); err != nil {
		t.Fatal(err)
	}
	thumbnailBytes, err := os.ReadFile(thumbnail)
	if err != nil {
		t.Fatal(err)
	}
	detailBytes, err := os.ReadFile(detail)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(thumbnailBytes, detailBytes) {
		t.Fatal("small cover variants should be byte-for-byte identical")
	}
}

func TestWebCoverGenerationHonorsCancelledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	directory := t.TempDir()
	err := generateWebCoverVariantsContext(
		ctx,
		filepath.Join(directory, "unused.jpg"),
		filepath.Join(directory, "thumbnail.webp"),
		filepath.Join(directory, "detail.webp"),
	)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled generation error = %v", err)
	}
}

func TestCoverDimensionSafetyLimit(t *testing.T) {
	if err := validateCoverDimensions(image.Config{Width: 4_000, Height: 6_000}); err != nil {
		t.Fatalf("ordinary high-resolution cover was rejected: %v", err)
	}
	for _, config := range []image.Config{
		{Width: 20_001, Height: 100},
		{Width: 10_000, Height: 10_000},
		{Width: 0, Height: 100},
	} {
		if err := validateCoverDimensions(config); err == nil {
			t.Fatalf("unsafe dimensions %dx%d were accepted", config.Width, config.Height)
		}
	}
}

func TestInvalidCoverIsOmittedInsteadOfCopiedToWebsite(t *testing.T) {
	paths := fixture(t)
	book := Normalize(Book{Title: "Malformed Cover"})
	book.CoverFile = book.ID + ".jpg"
	source := filepath.Join(paths.CoversDir, book.CoverFile)
	invalid := bytes.Repeat([]byte("not-an-image"), 1_024)
	if err := os.WriteFile(source, invalid, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := Save(paths, []Book{book}); err != nil {
		t.Fatal(err)
	}
	books, err := Load(paths)
	if err != nil {
		t.Fatal(err)
	}
	if err := SaveGenerated(paths, books); err != nil {
		t.Fatal(err)
	}

	generated, err := LoadGenerated(paths)
	if err != nil {
		t.Fatal(err)
	}
	if len(generated) != 1 || generated[0].Cover != "" || generated[0].Thumbnail != "" {
		t.Fatalf("invalid cover was published: %#v", generated)
	}
	for _, filename := range []string{
		filepath.Join(paths.PublicDir, "data", "covers", book.CoverFile),
		filepath.Join(paths.PublicDir, "data", "thumbnails", book.CoverFile),
		filepath.Join(paths.PublicDir, "data", "covers", generatedWebCoverFilename(book.CoverFile)),
		filepath.Join(paths.PublicDir, "data", "thumbnails", generatedWebCoverFilename(book.CoverFile)),
	} {
		if _, err := os.Stat(filename); !os.IsNotExist(err) {
			t.Fatalf("invalid published cover exists at %s: %v", filename, err)
		}
	}
	durable, err := os.ReadFile(source)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(durable, invalid) {
		t.Fatal("durable source cover was modified")
	}
	warnings, err := ValidationWarnings(paths, books)
	if err != nil {
		t.Fatal(err)
	}
	if joined := strings.Join(warnings, "\n"); !strings.Contains(joined, "cannot be published") {
		t.Fatalf("missing invalid-cover warning:\n%s", joined)
	}
}
