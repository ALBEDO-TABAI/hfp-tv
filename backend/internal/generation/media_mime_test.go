package generation

import "testing"

func TestNormalizedMediaMIMETypeNamesImagesByContent(t *testing.T) {
	jpeg := []byte("\xff\xd8\xff\xe0\x00\x10JFIF\x00\x01")
	for _, test := range []struct {
		declared string
		data     []byte
		want     string
	}{
		{"image/png", jpeg, "image/jpeg"},
		{"", jpeg, "image/jpeg"},
		{"video/mp4", jpeg, "video/mp4"},
		{"image/png", []byte("fixture"), "image/png"},
		{"", []byte("plain"), "text/plain"},
	} {
		if got := NormalizedMediaMIMEType(test.declared, test.data); got != test.want {
			t.Errorf("NormalizedMediaMIMEType(%q) = %q, want %q", test.declared, got, test.want)
		}
	}
}
