package kernelpool

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadRelayTemplateKeepsModelEndpointsOnTheRelay(t *testing.T) {
	dir := t.TempDir()
	write := func(body string) string {
		path := filepath.Join(dir, "relay.json")
		if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
		return path
	}
	ok := write(`{"baseUrl":"http://10.10.10.45:3000/v1","modelProfiles":[{"model":"seedance","baseUrl":"http://10.10.10.45:3000/doubao/api/v3"}]}`)
	if _, err := LoadRelayTemplate(ok); err != nil {
		t.Fatalf("same-origin model endpoint rejected: %v", err)
	}
	for name, body := range map[string]string{
		"other host":   `{"baseUrl":"http://10.10.10.45:3000/v1","modelProfiles":[{"model":"seedance","baseUrl":"http://10.10.10.46:3000/doubao/api/v3"}]}`,
		"other port":   `{"baseUrl":"http://10.10.10.45:3000/v1","modelProfiles":[{"model":"seedance","baseUrl":"http://10.10.10.45:8787/v1"}]}`,
		"other scheme": `{"baseUrl":"http://10.10.10.45:3000/v1","modelProfiles":[{"model":"seedance","baseUrl":"https://10.10.10.45:3000/v1"}]}`,
	} {
		if _, err := LoadRelayTemplate(write(body)); err == nil || !strings.Contains(err.Error(), "同协议") {
			t.Errorf("%s: %v", name, err)
		}
	}
}

func TestLoadRelayTemplateChecksReferenceServices(t *testing.T) {
	path := filepath.Join(t.TempDir(), "relay.json")
	write := func(body string) {
		if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	write(`{"baseUrl":"http://10.10.10.45:3000/v1","modelProfiles":[{"model":"a","referenceAssetService":"http://10.10.10.45:8787/v1/reference-images"},{"model":"b","referenceAssetService":"http://10.10.10.45:8787/v1/reference-images"}]}`)
	template, err := LoadRelayTemplate(path)
	if err != nil {
		t.Fatal(err)
	}
	if services := RelayReferenceServices(template); len(services) != 1 || services[0] != "http://10.10.10.45:8787/v1/reference-images" {
		t.Fatalf("services = %v", services)
	}
	write(`{"baseUrl":"http://10.10.10.45:3000/v1","modelProfiles":[{"model":"a","referenceAssetService":"10.10.10.45:8787"}]}`)
	if _, err := LoadRelayTemplate(path); err == nil || !strings.Contains(err.Error(), "referenceAssetService") {
		t.Fatalf("bad service accepted: %v", err)
	}
}
