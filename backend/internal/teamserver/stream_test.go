package teamserver

import (
	"bufio"
	"context"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"infinite-canvas/backend/internal/accounts"
)

// The kernel's SSE handler runs behind the gateway's gin engine; events must
// be flushed through both layers rather than buffered until the stream ends.
func TestGatewayStreamsCanvasEvents(t *testing.T) {
	h := newHarness(t)
	if _, err := h.users.CreateUser(context.Background(), "", accounts.CreateUserInput{Username: "streamer", Password: "stream-pass"}); err != nil {
		t.Fatal(err)
	}
	cookie := h.login("streamer", "stream-pass")
	save := func(revision int) {
		body := `{"project":{"id":"live","title":"Live","revision":` + strconv.Itoa(revision) + `,"nodes":[],"connections":[]}}`
		if response := h.do(call{method: http.MethodPut, path: "/api/canvas-projects/live", cookie: cookie, body: body}); response.Code != http.StatusOK {
			t.Fatalf("save = %d %s", response.Code, response.Body.String())
		}
	}
	save(0)

	server := httptest.NewServer(h.server.Handler())
	defer server.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	request, _ := http.NewRequestWithContext(ctx, http.MethodGet, server.URL+"/api/canvas-projects/live/events", nil)
	request.AddCookie(&http.Cookie{Name: SessionCookie, Value: cookie})
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK || !strings.HasPrefix(response.Header.Get("Content-Type"), "text/event-stream") {
		t.Fatalf("events = %d %s", response.StatusCode, response.Header.Get("Content-Type"))
	}
	lines := bufio.NewScanner(response.Body)
	waitFor := func(event string) {
		for lines.Scan() {
			if lines.Text() == "event: "+event {
				return
			}
		}
		t.Fatalf("stream ended before %s: %v", event, lines.Err())
	}
	waitFor("ready")
	save(1)
	waitFor("canvas.updated")
}
