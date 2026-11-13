package generation

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
)

type imageLinkPort struct {
	path        string
	contentType string
	body        map[string]any
}

func (p *imageLinkPort) Intercept(req *http.Request) (bool, []byte, string, error) {
	p.path, p.contentType = req.URL.Path, req.Header.Get("Content-Type")
	raw, _ := io.ReadAll(req.Body)
	_ = json.Unmarshal(raw, &p.body)
	return true, []byte(`{"data":[{"url":"https://files.example/out.jpg"}]}`), "application/json", nil
}

func imageLinkInput() Input {
	return Input{Mode: "image", Prompt: "merge", Config: Config{BaseURL: "https://8.8.8.8", APIKey: "synthetic", Model: "gpt-image-2.5", InterfaceType: "openai-image", Size: "3264x2448",
		ReferenceMediaTransport: map[string]string{"image": "url"}},
		ReferenceImages: []Media{{URL: "https://files.example/a.png"}, {URL: "https://files.example/b.png"}}}
}

// A model set to send reference images as links is submitted as a JSON edit
// with image_urls, even when the installed OpenAI Images package would upload files.
func TestImageEditSendsReferenceLinks(t *testing.T) {
	registry := LoadOfficialFallbackRegistry()
	for _, withRegistry := range []bool{false, true} {
		port := &imageLinkPort{}
		ctx := WithRuntime(context.Background(), Runtime{Images: port})
		if withRegistry {
			if _, ok := registry.Resolve("openai-image"); !ok {
				t.Skip("official protocol packages are not built")
			}
			ctx = WithProtocolRegistry(ctx, registry)
		}
		if _, err := RunImageTask(ctx, imageLinkInput()); err != nil {
			t.Fatal(err)
		}
		links, _ := json.Marshal(port.body["image_urls"])
		if port.path != "/v1/images/edits" || !strings.HasPrefix(port.contentType, "application/json") || string(links) != `["https://files.example/a.png","https://files.example/b.png"]` || port.body["size"] != "3264x2448" {
			t.Fatalf("registry=%v path=%s type=%s body=%#v", withRegistry, port.path, port.contentType, port.body)
		}
	}
}

func TestImageEditLinkModeRefusesWhatItCannotSend(t *testing.T) {
	ctx := WithRuntime(context.Background(), Runtime{Images: &imageLinkPort{}})
	local := imageLinkInput()
	local.ReferenceImages[1] = Media{DataURL: "data:image/png;base64,AA=="}
	if _, err := RunImageTask(ctx, local); err == nil || !strings.Contains(err.Error(), "参考图 2") {
		t.Fatalf("local image accepted: %v", err)
	}
	masked := imageLinkInput()
	masked.Mask = &Media{DataURL: "data:image/png;base64,AA=="}
	if _, err := RunImageTask(ctx, masked); err == nil || !strings.Contains(err.Error(), "蒙版") {
		t.Fatalf("mask accepted: %v", err)
	}
	// Without the setting the request keeps uploading files.
	port := &imageLinkPort{}
	upload := imageLinkInput()
	upload.Config.ReferenceMediaTransport = nil
	upload.ReferenceImages = []Media{{DataURL: "data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAwMCAO+jR1sAAAAASUVORK5CYII="}}
	if _, err := RunImageTask(WithRuntime(context.Background(), Runtime{Images: port}), upload); err != nil || !strings.HasPrefix(port.contentType, "multipart/form-data") {
		t.Fatalf("upload path changed: type=%s err=%v", port.contentType, err)
	}
}
