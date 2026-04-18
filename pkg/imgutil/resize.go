package imgutil

import (
	"bytes"
	"fmt"
	"image/jpeg"
	"io"

	"github.com/disintegration/imaging"
)

// Resize decodes an image from src, center-crops it to w×h, and returns JPEG-encoded bytes.
func Resize(src io.Reader, w, h int) ([]byte, string, error) {
	img, err := imaging.Decode(src, imaging.AutoOrientation(true))
	if err != nil {
		return nil, "", fmt.Errorf("decode image: %w", err)
	}

	resized := imaging.Fill(img, w, h, imaging.Center, imaging.Lanczos)

	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, resized, &jpeg.Options{Quality: 85}); err != nil {
		return nil, "", fmt.Errorf("encode jpeg: %w", err)
	}

	return buf.Bytes(), "image/jpeg", nil
}
