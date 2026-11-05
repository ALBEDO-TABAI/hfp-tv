package outbound

import (
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

type credentialRedirectTransport func(*http.Request) (*http.Response, error)

func (f credentialRedirectTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

// A provider may hand a result off to a CDN or another port. The redirect must
// still work, but no key header may follow it off the configured origin.
func TestCredentialHeadersStayOnTheirOriginAcrossRedirects(t *testing.T) {
	t.Setenv("CANVAS_ALLOW_PRIVATE_UPSTREAMS", "true")
	for _, header := range []string{"Authorization", "X-Goog-Api-Key", "X-Api-Key", "X-Access-Token"} {
		t.Run(header, func(t *testing.T) {
			seen := map[string]string{}
			client := OutboundHTTPClient(time.Second)
			client.Transport = credentialRedirectTransport(func(r *http.Request) (*http.Response, error) {
				seen[r.URL.Host+r.URL.Path] = r.Header.Get(header)
				status, location := 200, ""
				switch r.URL.Path {
				case "/local":
					status, location = 307, "/same"
				case "/cdn":
					status, location = 302, "http://127.0.0.1:9001/file"
				case "/scheme":
					status, location = 302, "https://127.0.0.1:9000/file"
				}
				return &http.Response{StatusCode: status, Header: http.Header{"Location": []string{location}}, Body: io.NopCloser(strings.NewReader("")), Request: r}, nil
			})
			for _, path := range []string{"/local", "/cdn", "/scheme"} {
				req, _ := http.NewRequest(http.MethodGet, "http://127.0.0.1:9000"+path, nil)
				req.Header.Set(header, "Bearer SYNTHETIC_CREDENTIAL")
				resp, err := client.Do(req)
				if err != nil {
					t.Fatalf("%s: redirect failed: %v", path, err)
				}
				resp.Body.Close()
			}
			if seen["127.0.0.1:9000/same"] == "" {
				t.Fatal("same-origin redirect lost the credential")
			}
			if seen["127.0.0.1:9001/file"] != "" || seen["127.0.0.1:9000/file"] != "" {
				t.Fatalf("credential followed an origin change: %v", seen)
			}
		})
	}
}

func TestSameOriginTreatsDefaultPortsAsEqual(t *testing.T) {
	parse := func(raw string) *http.Request { r, _ := http.NewRequest(http.MethodGet, raw, nil); return r }
	if !sameOrigin(parse("http://127.0.0.1:80/a").URL, parse("http://127.0.0.1/b").URL) {
		t.Fatal("default port should be the same origin")
	}
	for _, other := range []string{"https://127.0.0.1/a", "http://127.0.0.2/a", "http://127.0.0.1:81/a"} {
		if sameOrigin(parse(other).URL, parse("http://127.0.0.1/b").URL) {
			t.Fatalf("%s treated as the same origin", other)
		}
	}
}
