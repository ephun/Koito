package imagecache

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"testing"

	"golang.org/x/image/webp"
)

// Exercise the actual libvips binding and WebP encoder. This test must also run
// on Windows; a successful build alone does not prove DLL loading/encoding.
func TestCompressImage(t *testing.T) {
	input := image.NewNRGBA(image.Rect(0, 0, 1600, 1200))
	for y := 0; y < 1200; y++ {
		for x := 0; x < 1600; x++ {
			input.SetNRGBA(x, y, color.NRGBA{R: uint8(x), G: uint8(y), B: 100, A: 255})
		}
	}
	var pngBytes bytes.Buffer
	if err := png.Encode(&pngBytes, input); err != nil {
		t.Fatal(err)
	}
	for _, size := range []ImageSize{ImageSizeXS, ImageSizeSmall, ImageSizeMedium, ImageSizeLarge, ImageSizeXL, ImageSizeSource} {
		t.Run(string(size), func(t *testing.T) {
			result, err := compressImage(size, bytes.NewReader(pngBytes.Bytes()))
			if err != nil {
				t.Fatal(err)
			}
			decoded, err := webp.DecodeConfig(result)
			if err != nil {
				t.Fatal(err)
			}
			if decoded.Width != size.Width() || decoded.Height != size.Width() {
				t.Fatalf("got %dx%d, expected %dx%d", decoded.Width, decoded.Height, size.Width(), size.Width())
			}
		})
	}
}

func TestCompressImageRejectsInvalidImage(t *testing.T) {
	if _, err := compressImage(ImageSizeSmall, bytes.NewBufferString("not an image")); err == nil {
		t.Fatal("invalid image accepted")
	}
}
