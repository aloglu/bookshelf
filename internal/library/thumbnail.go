package library

import (
	"context"
	"errors"
	"fmt"
	"image"
	"io"
	"os"
	"path/filepath"
	"strings"

	gowebp "github.com/linzeyan/webp-go"
	"golang.org/x/image/draw"
)

type invalidCoverSourceError struct {
	err error
}

func (e *invalidCoverSourceError) Error() string {
	return e.err.Error()
}

func (e *invalidCoverSourceError) Unwrap() error {
	return e.err
}

func isInvalidCoverSource(err error) bool {
	var invalid *invalidCoverSourceError
	return errors.As(err, &invalid)
}

const (
	websiteThumbnailWidth = 360
	websiteDetailWidth    = 480
	websiteWebPQuality    = 82
	websiteWebPMethod     = 4
	maxCoverDimension     = 20_000
	maxCoverPixels        = 50_000_000
)

func generatedWebCoverFilename(sourceFilename string) string {
	return strings.TrimSuffix(sourceFilename, filepath.Ext(sourceFilename)) + ".webp"
}

func generateWebCoverVariants(source, thumbnailDestination, detailDestination string) error {
	return generateWebCoverVariantsContext(context.Background(), source, thumbnailDestination, detailDestination)
}

func generateWebCoverVariantsContext(ctx context.Context, source, thumbnailDestination, detailDestination string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	input, err := os.Open(source)
	if err != nil {
		return err
	}
	decoded, _, err := decodeCoverImage(input)
	closeErr := input.Close()
	if err != nil {
		return &invalidCoverSourceError{err: fmt.Errorf("decode website cover source: %w", err)}
	}
	if closeErr != nil {
		return closeErr
	}
	bounds := decoded.Bounds()
	if bounds.Dx() <= 0 || bounds.Dy() <= 0 {
		return &invalidCoverSourceError{err: fmt.Errorf("cover has invalid dimensions")}
	}

	thumbnail := resizeWebCover(decoded, websiteThumbnailWidth)
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := encodeWebCoverContext(ctx, thumbnailDestination, thumbnail); err != nil {
		return fmt.Errorf("encode website cover thumbnail: %w", err)
	}
	if bounds.Dx() <= websiteThumbnailWidth {
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := copyFile(thumbnailDestination, detailDestination); err != nil {
			return fmt.Errorf("copy website detail cover: %w", err)
		}
		return nil
	}
	detail := resizeWebCover(decoded, websiteDetailWidth)
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := encodeWebCoverContext(ctx, detailDestination, detail); err != nil {
		return fmt.Errorf("encode website detail cover: %w", err)
	}
	return nil
}

func decodeCoverImage(input io.ReadSeeker) (image.Image, string, error) {
	config, format, err := image.DecodeConfig(input)
	if err != nil {
		return nil, "", err
	}
	if err := validateCoverDimensions(config); err != nil {
		return nil, "", err
	}
	if _, err := input.Seek(0, io.SeekStart); err != nil {
		return nil, "", err
	}
	decoded, decodedFormat, err := image.Decode(input)
	if decodedFormat != "" {
		format = decodedFormat
	}
	return decoded, format, err
}

func validateCoverDimensions(config image.Config) error {
	if config.Width <= 0 || config.Height <= 0 {
		return fmt.Errorf("cover has invalid dimensions %dx%d", config.Width, config.Height)
	}
	if config.Width > maxCoverDimension || config.Height > maxCoverDimension ||
		int64(config.Width)*int64(config.Height) > maxCoverPixels {
		return fmt.Errorf(
			"cover dimensions %dx%d exceed the safety limit of %d pixels and %d pixels per side",
			config.Width, config.Height, maxCoverPixels, maxCoverDimension,
		)
	}
	return nil
}

func resizeWebCover(source image.Image, maximumWidth int) image.Image {
	bounds := source.Bounds()
	width, height := bounds.Dx(), bounds.Dy()
	targetWidth := min(width, maximumWidth)
	targetHeight := max(1, height*targetWidth/width)
	target := image.NewRGBA(image.Rect(0, 0, targetWidth, targetHeight))
	draw.CatmullRom.Scale(target, target.Bounds(), source, bounds, draw.Over, nil)
	return target
}

func encodeWebCover(destination string, source image.Image) error {
	return encodeWebCoverContext(context.Background(), destination, source)
}

func encodeWebCoverContext(ctx context.Context, destination string, source image.Image) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(destination), 0o755); err != nil {
		return err
	}
	output, err := os.OpenFile(destination, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	encodeErr := gowebp.EncodeContext(ctx, output, source, &gowebp.Options{
		Lossy:   true,
		Quality: websiteWebPQuality,
		Method:  websiteWebPMethod,
	})
	closeErr := output.Close()
	if encodeErr != nil {
		_ = os.Remove(destination)
		return encodeErr
	}
	if closeErr != nil {
		_ = os.Remove(destination)
	}
	return closeErr
}
