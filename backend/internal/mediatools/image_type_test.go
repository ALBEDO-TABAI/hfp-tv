package mediatools

import "testing"

func TestContentImageMIMETypeTrustsTheBytesOverAnImageLabel(t *testing.T) {
	jpeg := []byte("\xff\xd8\xff\xe0\x00\x10JFIF\x00\x01")
	png := []byte("\x89PNG\r\n\x1a\n\x00\x00\x00\rIHDR")
	gif := []byte("GIF89a\x01\x00\x01\x00")
	webp := []byte("RIFF\x24\x00\x00\x00WEBPVP8 ")
	for _, test := range []struct {
		label string
		data  []byte
		want  string
	}{
		{"image/png", jpeg, "image/jpeg"},
		{"image/jpeg; charset=binary", png, "image/png"},
		{"IMAGE/PNG", webp, "image/webp"},
		{"", gif, "image/gif"},
		{"application/octet-stream", jpeg, "image/jpeg"},
		// A non-image label is never overridden, and unknown bytes keep the label.
		{"video/mp4", jpeg, ""},
		{"image/png", []byte("not an image"), ""},
		{"image/svg+xml", []byte(`<svg xmlns="http://www.w3.org/2000/svg"/>`), ""},
		{"image/png", nil, ""},
	} {
		if got := ContentImageMIMEType(test.label, test.data); got != test.want {
			t.Errorf("ContentImageMIMEType(%q, %q) = %q, want %q", test.label, test.data, got, test.want)
		}
	}
}
