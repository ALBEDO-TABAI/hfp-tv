package generation

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

// notingReceipts records the task-log lines the poll observer writes.
type notingReceipts struct {
	recordingReceipts
	mu    sync.Mutex
	notes []string
}

func (r *notingReceipts) NotePoll(_ context.Context, level, message string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.notes = append(r.notes, level+" "+message)
}

func (r *notingReceipts) joined() string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return strings.Join(r.notes, "\n")
}

// A New API relay answers the Ark Seedance protocol with its own task shapes:
// a bare {id} before its poller has the state, its task record under data,
// then the OpenAI video object whose address is metadata.url.
func TestSeedanceThroughNewAPIRelayShapesIsRetrievedAndLogged(t *testing.T) {
	payload := unifiedVideoPayload(t)
	media := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write(payload) }))
	defer media.Close()
	answers := []any{
		map[string]any{"id": "task_abc"},
		map[string]any{"code": "success", "message": "", "data": map[string]any{"task_id": "task_abc", "status": "IN_PROGRESS", "progress": "50%"}},
		map[string]any{"id": "task_abc", "object": "video", "status": "completed", "progress": 100, "metadata": map[string]any{"url": media.URL + "/video.mp4?signature=x"}},
	}
	polls := 0
	relay := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || !strings.HasSuffix(r.URL.Path, "/contents/generations/tasks/task_abc") {
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		_ = json.NewEncoder(w).Encode(answers[min(polls, len(answers)-1)])
		polls++
	}))
	defer relay.Close()
	receipts := &notingReceipts{}
	policy := DefaultVideoPollPolicy()
	policy.Sleep = func(context.Context, time.Duration) error { return nil }
	ctx := WithCallMeta(WithRuntime(context.Background(), Runtime{Receipts: receipts}), CallMeta{ProviderRequestID: "task_abc"})
	result, err := RunProtocolAdapterTaskWithPolicy(ctx, Input{Mode: "video", Config: Config{BaseURL: relay.URL + "/doubao/api/v3", APIKey: "member-key", InterfaceType: "volcengine-ark-video", Model: "doubao-seedance-2.0-fast"}}, unifiedVideoAdapter(t, "volcengine-ark-seedance"), policy)
	if err != nil {
		t.Fatalf("%v\nnotes:\n%s", err, receipts.joined())
	}
	unifiedVideoResult(t, result, payload)
	notes := receipts.joined()
	for _, want := range []string{"暂时没有任务状态", "上游任务状态：IN_PROGRESS，进度 50%", "上游任务状态：completed，进度 100%"} {
		if !strings.Contains(notes, want) {
			t.Fatalf("task log is missing %q:\n%s", want, notes)
		}
	}
	if strings.Contains(notes, "signature") || strings.Contains(notes, media.URL) {
		t.Fatalf("task log leaked the result address:\n%s", notes)
	}
}

func TestRelayFailureNamesTheUpstreamStatusAndReason(t *testing.T) {
	relay := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"code": "success", "data": map[string]any{"task_id": "task_abc", "status": "FAILURE", "fail_reason": "视频结果拉取超时"}})
	}))
	defer relay.Close()
	policy := DefaultVideoPollPolicy()
	policy.Sleep = func(context.Context, time.Duration) error { return nil }
	ctx := WithCallMeta(context.Background(), CallMeta{ProviderRequestID: "task_abc"})
	_, err := RunProtocolAdapterTaskWithPolicy(ctx, Input{Mode: "video", Config: Config{BaseURL: relay.URL + "/doubao/api/v3", APIKey: "member-key", InterfaceType: "volcengine-ark-video", Model: "doubao-seedance-2.0-fast"}}, unifiedVideoAdapter(t, "volcengine-ark-seedance"), policy)
	if err == nil || !strings.Contains(err.Error(), "视频结果拉取超时") || !strings.Contains(err.Error(), "FAILURE") {
		t.Fatalf("failure = %v", err)
	}
}
