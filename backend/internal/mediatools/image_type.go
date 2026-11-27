package mediatools

import (
	"net/http"
	"strings"
)

// ContentImageMIMEType names a PNG, JPEG, GIF or WebP image by its leading
// bytes when its label allows an image (an image type, none, or
// application/octet-stream), and returns "" otherwise. Labels come from file
// extensions, response headers or a provider's default and are often wrong (a
// JPEG served as image/png); strict receivers such as the company
// reference-image service compare the declared type with the signature.
func ContentImageMIMEType(label string, data []byte) string {
	label = strings.ToLower(strings.TrimSpace(strings.Split(label, ";")[0]))
	if label != "" && label != "application/octet-stream" && !strings.HasPrefix(label, "image/") {
		return ""
	}
	switch detected := strings.TrimSpace(strings.Split(http.DetectContentType(data), ";")[0]); detected {
	case "image/png", "image/jpeg", "image/gif", "image/webp":
		return detected
	}
	return ""
}
