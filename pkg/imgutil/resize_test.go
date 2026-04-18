package imgutil

import (
	"bytes"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestResize(t *testing.T) {
	tests := []struct {
		name      string
		src       func(t *testing.T) *bytes.Buffer
		w, h      int
		wantMime  string
		wantErr   string
	}{
		{
			name:     "square JPEG to 100x100",
			src:      func(t *testing.T) *bytes.Buffer { return encodeJPEG(t, 400, 400) },
			w:        100,
			h:        100,
			wantMime: "image/jpeg",
		},
		{
			name:     "rectangular PNG to 300x300 center-crop",
			src:      func(t *testing.T) *bytes.Buffer { return encodePNG(t, 800, 400) },
			w:        300,
			h:        300,
			wantMime: "image/jpeg",
		},
		{
			name:     "tall portrait to 100x100",
			src:      func(t *testing.T) *bytes.Buffer { return encodeJPEG(t, 200, 600) },
			w:        100,
			h:        100,
			wantMime: "image/jpeg",
		},
		{
			name:     "upscale small image",
			src:      func(t *testing.T) *bytes.Buffer { return encodePNG(t, 50, 50) },
			w:        300,
			h:        300,
			wantMime: "image/jpeg",
		},
		{
			name:    "garbage input returns error",
			src:     func(t *testing.T) *bytes.Buffer { return bytes.NewBufferString("not an image") },
			w:       100,
			h:       100,
			wantErr: "decode image",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data, mime, err := Resize(tt.src(t), tt.w, tt.h)

			if tt.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
				assert.Nil(t, data)
				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.wantMime, mime)
			assert.NotEmpty(t, data)

			decoded, err := jpeg.Decode(bytes.NewReader(data))
			require.NoError(t, err)
			bounds := decoded.Bounds()
			assert.Equal(t, tt.w, bounds.Dx())
			assert.Equal(t, tt.h, bounds.Dy())
		})
	}
}

func TestResize_EmptyReader(t *testing.T) {
	data, _, err := Resize(strings.NewReader(""), 100, 100)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "decode image")
	assert.Nil(t, data)
}

func encodeJPEG(t *testing.T, w, h int) *bytes.Buffer {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := range h {
		for x := range w {
			img.Set(x, y, color.RGBA{R: uint8(x % 256), G: uint8(y % 256), B: 100, A: 255})
		}
	}
	var buf bytes.Buffer
	require.NoError(t, jpeg.Encode(&buf, img, nil))
	return &buf
}

func encodePNG(t *testing.T, w, h int) *bytes.Buffer {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := range h {
		for x := range w {
			img.Set(x, y, color.RGBA{R: 50, G: uint8(x % 256), B: uint8(y % 256), A: 255})
		}
	}
	var buf bytes.Buffer
	require.NoError(t, png.Encode(&buf, img))
	return &buf
}
