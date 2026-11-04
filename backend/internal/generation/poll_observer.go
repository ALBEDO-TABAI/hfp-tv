package generation

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"sort"
	"strings"
	"time"

	"infinite-canvas/backend/internal/protocol"
)

// A relay that cannot read a task from its own upstream keeps answering
// "queued, 0%" and later fails the task without a reason (New API gives up
// after 20 failed queries, about 12 minutes), even though the video was made
// upstream. These thresholds decide when the task log calls that out.
const (
	pollStallNotice         = 5 * time.Minute
	pollStallFailureMinimum = 3 * time.Minute
)

// PollNoter is an optional ReceiptPort extension that writes a line to the
// task log. Poll observations go there so a member (or the administrator
// reading over their shoulder) can see what the relay actually answered.
type PollNoter interface {
	NotePoll(ctx context.Context, level, message string)
}

// pollObserver reports changes of the upstream task state while polling:
// the raw status and progress, a missing or unrecognized status, and a
// finished task without a usable video address. It records field names and
// status words only, never URLs, IDs of other systems or credentials.
type pollObserver struct {
	lastStatus   string
	lastProgress int
	noStatus     bool
	unknown      map[string]bool
	// taskID names the upstream task in stall messages; callers keep it current.
	taskID     string
	now        func() time.Time
	started    time.Time
	advanced   bool
	stallNoted bool
}

func newPollObserver(now func() time.Time) *pollObserver {
	if now == nil {
		now = time.Now
	}
	return &pollObserver{lastProgress: -1, unknown: map[string]bool{}, now: now, started: now()}
}

func (o *pollObserver) note(ctx context.Context, level, message string) {
	runtime, _ := RuntimeFromContext(ctx)
	if noter, ok := runtime.Receipts.(PollNoter); ok {
		noter.NotePoll(ctx, level, message)
	}
}

func (o *pollObserver) observe(ctx context.Context, body []byte) {
	var payload map[string]any
	if json.Unmarshal(body, &payload) != nil {
		return
	}
	o.observePayload(ctx, payload)
}

func (o *pollObserver) observePayload(ctx context.Context, payload map[string]any) {
	status := pollStatusText(payload)
	progress, hasProgress := pollProgress(payload)
	o.trackAdvance(status, progress, hasProgress)
	defer o.noteStall(ctx, status)
	if status == "" {
		if !o.noStatus {
			o.noStatus = true
			o.note(ctx, "info", "上游查询结果暂时没有任务状态，继续等待（返回字段："+pollFieldNames(payload)+"）")
		}
		return
	}
	if !protocol.KnownStatus(status) && !o.unknown[status] {
		o.unknown[status] = true
		o.note(ctx, "warn", fmt.Sprintf("上游返回了未识别的任务状态“%s”，按进行中继续查询（返回字段：%s）", status, pollFieldNames(payload)))
	}
	if status == o.lastStatus && (!hasProgress || progress == o.lastProgress) {
		return
	}
	o.lastStatus = status
	message := "上游任务状态：" + status
	if hasProgress {
		o.lastProgress = progress
		message += fmt.Sprintf("，进度 %d%%", progress)
	}
	o.note(ctx, "info", message)
}

// trackAdvance remembers whether the upstream ever reported anything beyond
// waiting at 0%. A failure is not progress: it is judged against what came
// before it.
func (o *pollObserver) trackAdvance(status string, progress int, hasProgress bool) {
	switch protocol.NormalizeStatus(status) {
	case protocol.StatusFailed, protocol.StatusCancelled:
		return
	case "", protocol.StatusPending:
		if hasProgress && progress > 0 {
			o.advanced = true
		}
	default:
		o.advanced = true
	}
}

// noteStall writes one warning when the upstream has said nothing but
// "waiting, 0%" for pollStallNotice. It does not fail the task: queues can be
// long, and the relay may still recover.
func (o *pollObserver) noteStall(ctx context.Context, status string) {
	waited := o.now().Sub(o.started)
	if o.advanced || o.stallNoted || waited < pollStallNotice {
		return
	}
	switch protocol.NormalizeStatus(status) {
	case "", protocol.StatusPending:
	default:
		return
	}
	o.stallNoted = true
	if status == "" {
		status = "没有状态"
	}
	o.note(ctx, "warn", fmt.Sprintf("上游任务已连续 %d 分钟停在“%s”、进度 0%%，还没有开始生成。若中转站后台显示视频已完成，说明中转站没能把结果同步到这条任务，请管理员按任务号%s在中转站日志里查原因", int(waited.Minutes()), status, o.taskLabel()))
}

// unexplainedFailure describes a failure that came without a reason after
// the upstream had only ever reported waiting at 0%: the shape of a relay
// that lost track of its upstream task. It returns "" for any other failure.
func (o *pollObserver) unexplainedFailure(reason string) string {
	waited := o.now().Sub(o.started)
	if o.advanced || strings.TrimSpace(reason) != "" || waited < pollStallFailureMinimum {
		return ""
	}
	return fmt.Sprintf("上游判定失败但没有给出原因：此前 %d 分钟里它一直显示排队中、进度 0%%，从未开始生成。若中转站后台显示该视频已完成，说明中转站没能从它的上游取回任务状态（New API 连续查询上游失败约 20 次后会自动判失败），请管理员按任务号%s在中转站日志里查原因", int(waited.Minutes()), o.taskLabel())
}

func (o *pollObserver) taskLabel() string {
	if id := strings.TrimSpace(o.taskID); id != "" {
		return " " + id + " "
	}
	return ""
}

// noteMissingVideo explains a finished task whose answer has no video address
// this host can read.
func (o *pollObserver) noteMissingVideo(ctx context.Context, body []byte) {
	var payload map[string]any
	if json.Unmarshal(body, &payload) != nil {
		return
	}
	o.note(ctx, "warn", "上游已完成，但返回内容里没有可识别的视频地址（返回字段："+pollFieldNames(payload)+"）")
}

func pollStatusText(payload map[string]any) string {
	for _, container := range []map[string]any{payload, objectValue(payload["data"]), objectValue(payload["task"])} {
		for _, key := range []string{"status", "state", "task_status"} {
			if value, ok := container[key].(string); ok && strings.TrimSpace(value) != "" {
				return strings.TrimSpace(value)
			}
		}
	}
	return ""
}

func pollProgress(payload map[string]any) (int, bool) {
	for _, container := range []map[string]any{payload, objectValue(payload["data"])} {
		switch value := container["progress"].(type) {
		case float64:
			if value >= 0 && value <= 100 {
				return int(value + 0.5), true
			}
		case string:
			var number float64
			if _, err := fmt.Sscanf(strings.TrimSuffix(strings.TrimSpace(value), "%"), "%g", &number); err == nil && number >= 0 && number <= 100 {
				return int(number + 0.5), true
			}
		}
	}
	return 0, false
}

// pollFieldNames lists the answer's field names (and those of a nested data
// object) so a format mismatch can be diagnosed from the task log.
func pollFieldNames(payload map[string]any) string {
	names := make([]string, 0, len(payload))
	for key, value := range payload {
		names = append(names, key)
		if key == "data" {
			for nested := range objectValue(value) {
				names = append(names, "data."+nested)
			}
		}
	}
	sort.Strings(names)
	if len(names) > 24 {
		names = append(names[:24], "…")
	}
	if len(names) == 0 {
		return "无"
	}
	return strings.Join(names, ", ")
}

// relayVideoReferences reads a finished video's address from the shapes New
// API relays use besides the provider-native ones a protocol declares: the
// OpenAI video object (metadata.url), the task record (result_url, or the
// legacy fail_reason that carries the URL on success) and the raw upstream
// task kept under data.
func relayVideoReferences(body []byte) []protocol.MediaReference {
	var payload map[string]any
	if json.Unmarshal(body, &payload) != nil {
		return nil
	}
	data := objectValue(payload["data"])
	candidates := []string{
		stringAt(objectValue(payload["metadata"]), "url"),
		stringAt(payload, "result_url"),
		stringAt(data, "result_url"),
		stringAt(objectValue(data["metadata"]), "url"),
		stringAt(objectValue(payload["content"]), "video_url"),
		stringAt(objectValue(data["content"]), "video_url"),
		stringAt(objectValue(objectValue(data["data"])["content"]), "video_url"),
		stringAt(data, "video_url"),
		stringAt(data, "fail_reason"),
		stringAt(payload, "fail_reason"),
	}
	for _, candidate := range candidates {
		if isHTTPMediaURL(candidate) {
			return []protocol.MediaReference{{URL: candidate, Kind: "video", Ephemeral: true}}
		}
	}
	return nil
}

func isHTTPMediaURL(value string) bool {
	parsed, err := url.Parse(strings.TrimSpace(value))
	return err == nil && (parsed.Scheme == "https" || parsed.Scheme == "http") && parsed.Host != "" && parsed.User == nil
}

func objectValue(value any) map[string]any {
	object, _ := value.(map[string]any)
	return object
}

func stringAt(object map[string]any, key string) string {
	value, _ := object[key].(string)
	return strings.TrimSpace(value)
}
