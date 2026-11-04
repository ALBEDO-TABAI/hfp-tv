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

// stuckRelayRun polls a relay that answers the given replies in turn (the last
// one repeats) on a virtual clock, the way a 12-minute relay run unfolds.
func stuckRelayRun(t *testing.T, replies []map[string]any) (*notingReceipts, error) {
	t.Helper()
	polls := 0
	relay := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(replies[min(polls, len(replies)-1)])
		polls++
	}))
	defer relay.Close()
	receipts := &notingReceipts{}
	clock := time.Date(2026, 11, 4, 11, 54, 31, 0, time.UTC)
	policy := DefaultVideoPollPolicy()
	policy.Now = func() time.Time { return clock }
	policy.Sleep = func(_ context.Context, wait time.Duration) error { clock = clock.Add(wait); return nil }
	ctx := WithCallMeta(WithRuntime(context.Background(), Runtime{Receipts: receipts}), CallMeta{ProviderRequestID: "task_Oy5P"})
	_, err := RunProtocolAdapterTaskWithPolicy(ctx, Input{Mode: "video", Config: Config{BaseURL: relay.URL + "/doubao/api/v3", APIKey: "member-key", InterfaceType: "volcengine-ark-video", Model: "doubao-seedance-2.0-fast"}}, unifiedVideoAdapter(t, "volcengine-ark-seedance"), policy)
	return receipts, err
}

func repeatReply(reply map[string]any, times int) []map[string]any {
	replies := make([]map[string]any, times)
	for index := range replies {
		replies[index] = reply
	}
	return replies
}

var (
	relayQueued = map[string]any{"id": "task_Oy5P", "object": "video", "status": "queued", "progress": 0}
	relayFailed = map[string]any{"id": "task_Oy5P", "object": "video", "status": "failed", "progress": 100, "error": map[string]any{"message": "", "code": ""}}
)

// The company relay of 2026-11-04: it never got the task's state from its
// own upstream, answered "queued, 0%" for 11 minutes, then failed the task
// with no reason while the video had been made upstream in 93 seconds.
func TestRelayThatNeverStartsExplainsItsReasonlessFailure(t *testing.T) {
	receipts, err := stuckRelayRun(t, append(repeatReply(relayQueued, 23), relayFailed))
	if err == nil {
		t.Fatal("a failed relay task succeeded")
	}
	for _, want := range []string{"task_Oy5P", "上游判定失败但没有给出原因", "此前 12 分钟里它一直显示排队中", "按任务号 task_Oy5P 在中转站日志里查原因", "（上游状态 failed）"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("failure is missing %q: %v", want, err)
		}
	}
	notes := receipts.joined()
	if strings.Count(notes, "上游任务已连续") != 1 || !strings.Contains(notes, "warn 上游任务已连续 5 分钟停在“queued”、进度 0%") || !strings.Contains(notes, "task_Oy5P") {
		t.Fatalf("task log should warn once about the stall:\n%s", notes)
	}
}

func TestRelayFailureWithAReasonOrProgressKeepsItsOwnMessage(t *testing.T) {
	withReason := map[string]any{"id": "task_Oy5P", "status": "failed", "progress": 100, "error": map[string]any{"message": "输入图片未通过审核"}}
	running := map[string]any{"id": "task_Oy5P", "object": "video", "status": "in_progress", "progress": 30}
	cases := map[string][]map[string]any{
		"reason given":          append(repeatReply(relayQueued, 23), withReason),
		"upstream had started":  append(append(repeatReply(relayQueued, 3), repeatReply(running, 20)...), relayFailed),
		"failed within minutes": append(repeatReply(relayQueued, 2), relayFailed),
	}
	for name, replies := range cases {
		t.Run(name, func(t *testing.T) {
			receipts, err := stuckRelayRun(t, replies)
			if err == nil {
				t.Fatal("a failed relay task succeeded")
			}
			if strings.Contains(err.Error(), "没有给出原因") {
				t.Fatalf("failure was rewritten: %v", err)
			}
			if name == "reason given" && !strings.Contains(err.Error(), "输入图片未通过审核") {
				t.Fatalf("failure lost the upstream reason: %v", err)
			}
			if name == "upstream had started" && strings.Contains(receipts.joined(), "上游任务已连续") {
				t.Fatalf("a running task was reported as stuck:\n%s", receipts.joined())
			}
		})
	}
}
