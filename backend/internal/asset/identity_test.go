package asset

import (
	"bytes"
	"io"
	"testing"
)

// Browsers label a file by its extension, so a JPEG saved as .png arrives as
// image/png.
func TestUploadedImagesAreNamedByTheirBytes(t *testing.T) {
	jpeg := []byte("\xff\xd8\xff\xe0\x00\x10JFIF\x00\x01")
	file := bytes.NewReader(jpeg)
	if got := DetectUploadedMimeType(file, "photo.png", "image/png"); got != "image/jpeg" {
		t.Fatalf("jpeg labelled png = %q", got)
	}
	if rest, _ := io.ReadAll(file); !bytes.Equal(rest, jpeg) {
		t.Fatal("the upload was not rewound after sniffing")
	}
	svg := []byte(`<svg xmlns="http://www.w3.org/2000/svg"/>`)
	for _, test := range []struct {
		file     io.ReadSeeker
		name     string
		declared string
		want     string
	}{
		{bytes.NewReader(svg), "icon.svg", "image/svg+xml", "image/svg+xml"},
		{bytes.NewReader(jpeg), "clip.mp4", "video/mp4", "video/mp4"},
		{bytes.NewReader(jpeg), "photo", "", "image/jpeg"},
		{nil, "photo.png", "", "image/png"},
	} {
		if got := DetectUploadedMimeType(test.file, test.name, test.declared); got != test.want {
			t.Errorf("DetectUploadedMimeType(%q, %q) = %q, want %q", test.name, test.declared, got, test.want)
		}
	}
}
