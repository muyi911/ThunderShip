package core

import (
	"bytes"
	"encoding/base64"
	"image"
	"image/png"
	"strings"
	"testing"
)

func TestValidateAIImages(t *testing.T) {
	var b bytes.Buffer
	if err := png.Encode(&b, image.NewRGBA(image.Rect(0, 0, 2, 2))); err != nil {
		t.Fatal(err)
	}
	valid := "data:image/png;base64," + base64.StdEncoding.EncodeToString(b.Bytes())
	for _, images := range [][]string{nil, {valid}, {valid, valid, valid, valid}} {
		if err := ValidateAIImages(images); err != nil {
			t.Fatal(err)
		}
	}
	for _, images := range [][]string{
		{valid, valid, valid, valid, valid},
		{"https://example.com/image.png"},
		{"data:image/png;base64,???"},
		{"data:image/png;base64," + base64.StdEncoding.EncodeToString([]byte("not an image"))},
		{strings.Replace(valid, "image/png", "image/jpeg", 1)},
		{"data:image/png;base64," + strings.Repeat("A", base64.StdEncoding.EncodedLen(8*1024*1024)+1)},
	} {
		if err := ValidateAIImages(images); err == nil {
			t.Fatal("invalid attachment accepted")
		}
	}
}
