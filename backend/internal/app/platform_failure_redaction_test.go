package app

import (
	"encoding/json"
	"errors"
	"infinite-canvas/backend/internal/model"
	"strings"
	"testing"
)

func TestReferenceFailureRedactsActualKeyBeforeDiagnosticsAndReport(t *testing.T) {
	const key = "FAKE_CREDENTIAL_123456"
	err := referenceServiceFailure("synthetic-request", key, false, errors.New("reference rejected "+key))
	task := model.Task{ID: "synthetic-task", Status: model.TaskStatusFailed, Error: err.Error()}
	captureTaskFailureDiagnostics(&task, err, "unknown")
	report := (&Service{}).taskFailureReport(task)
	encoded, marshalErr := json.Marshal([]any{task.FailureDiagnostics, report, err.Error()})
	if marshalErr != nil {
		t.Fatal(marshalErr)
	}
	if strings.Contains(string(encoded), key) {
		t.Fatal("actual credential reached diagnostics or platform report")
	}
	if !strings.Contains(task.FailureDiagnostics.Summary, "reference rejected") {
		t.Fatal("redaction lost useful diagnosis")
	}
}

// A provider error can echo the request's key in any format; the report that
// leaves the workspace for the administrators must not carry it.
func TestFailureReportRemovesTheTasksOwnKeys(t *testing.T) {
	const key = "FAKE_CREDENTIAL_ABCDEFGH"
	const header = "HEADER_SECRET_12345678"
	service, _, _, _ := creationTestService(t)
	input := map[string]any{"config": map[string]any{"apiKey": key, "headers": []map[string]string{{"name": "Authorization", "value": "Bearer " + header}}}}
	if err := service.protectTaskSecrets(input); err != nil {
		t.Fatal(err)
	}
	encodedInput, _ := json.Marshal(input)
	if strings.Contains(string(encodedInput), key) {
		t.Fatal("task input stored the key in clear")
	}
	task := model.Task{ID: "synthetic-task", Status: model.TaskStatusFailed, InputJSON: string(encodedInput),
		Error: "上游拒绝：invalid token " + key, FailureDiagnostics: &model.TaskFailureDiagnostics{Summary: "upstream said " + header + " is wrong"}}
	report := service.taskFailureReport(task)
	encoded, _ := json.Marshal(report)
	if strings.Contains(string(encoded), key) || strings.Contains(string(encoded), header) || !strings.Contains(report.Message, "invalid token [REDACTED]") {
		t.Fatalf("report kept a key: %s", encoded)
	}
}

// Execution can refresh a company relay task's key after the failing copy was
// read; the report must also remove the key the request actually carried, and
// an upstream task id that echoes it.
func TestFailureReportUsesTheStoredRefreshedKey(t *testing.T) {
	const oldKey = "OLD_SYNTHETIC_KEY_1234"
	const newKey = "NEW_SYNTHETIC_KEY_5678"
	service, db, _, _ := creationTestService(t)
	encode := func(key string) string {
		input := map[string]any{"config": map[string]any{"apiKey": key}}
		if err := service.protectTaskSecrets(input); err != nil {
			t.Fatal(err)
		}
		raw, _ := json.Marshal(input)
		return string(raw)
	}
	stored := model.Task{ID: "refreshed-task", UserID: "owner", Type: "canvas_image", Status: model.TaskStatusFailed, InputJSON: encode(newKey)}
	if err := db.Create(&stored).Error; err != nil {
		t.Fatal(err)
	}
	stale := stored
	stale.InputJSON = encode(oldKey)
	stale.Error = "rejected " + newKey
	stale.ProviderRequestID = "task-" + newKey
	report := service.taskFailureReport(stale)
	encoded, _ := json.Marshal(report)
	if strings.Contains(string(encoded), newKey) {
		t.Fatalf("report kept the refreshed key: %s", encoded)
	}
}

// When the task's keys cannot be read, upstream text is left out of the report.
func TestFailureReportDropsUpstreamTextWhenKeysAreUnknown(t *testing.T) {
	service, _, _, _ := creationTestService(t)
	task := model.Task{ID: "unreadable", Status: model.TaskStatusFailed, InputJSON: `{"config":`, Error: "生成失败",
		FailureDiagnostics: &model.TaskFailureDiagnostics{Summary: "upstream echoed something"}}
	report := service.taskFailureReport(task)
	if report.UpstreamDetail != "" || len(report.Log) != 0 || strings.Contains(report.Message, "upstream") || !strings.Contains(report.Message, "未上报详情") {
		t.Fatalf("report kept unverified upstream text: %+v", report)
	}
}
