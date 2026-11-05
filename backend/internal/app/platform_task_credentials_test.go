package app

import (
	"context"
	"encoding/json"
	"fmt"
	"infinite-canvas/backend/internal/model"
	"strings"
	"testing"
	"time"
)

func TestPlatformTaskRetryUsesCurrentBinding(t *testing.T) {
	for _, key := range []string{"SYNTHETIC_NEW", ""} {
		t.Run("key="+key, func(t *testing.T) {
			s, db, _, _ := creationTestService(t)
			s.SetPlatformChannels([]string{"company-relay"})
			save := func(key, base string) {
				t.Helper()
				raw, _ := json.Marshal(map[string]any{"channels": []any{map[string]any{"id": "company-relay", "apiKey": key, "baseUrl": base}}})
				if err := s.SaveLocalModelConfig(raw); err != nil {
					t.Fatal(err)
				}
			}
			save("SYNTHETIC_OLD", "http://127.0.0.1:9000/v1")
			input, err := s.resolvePlatformChannelSecrets(map[string]any{"mode": "text", "prompt": "test", "config": map[string]any{"channelId": "company-relay", "model": "text-test", "interfaceType": "chat-completion"}})
			if err != nil {
				t.Fatal(err)
			}
			if err := s.protectTaskSecrets(input); err != nil {
				t.Fatal(err)
			}
			raw, _ := json.Marshal(input)
			task := model.Task{ID: "retry-current", UserID: "user", Type: "canvas_text", Model: "text-test", Status: model.TaskStatusFailed, Error: "HTTP 429", InputJSON: string(raw)}
			if err := db.Create(&task).Error; err != nil {
				t.Fatal(err)
			}
			save(key, "http://127.0.0.1:9001/v1")
			_, err = s.RetryTask(task.UserID, task.ID)
			if key == "" {
				if err == nil || !strings.Contains(err.Error(), "管理员还没有为你绑定") {
					t.Fatalf("cleared binding retry = %v", err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			stored, err := s.repo.Task(task.ID)
			if err != nil {
				t.Fatal(err)
			}
			plain, err := s.decryptTaskInputJSON(stored.InputJSON)
			if err != nil || !strings.Contains(plain, key) || strings.Contains(plain, "SYNTHETIC_OLD") || !strings.Contains(plain, ":9001/") || strings.Contains(stored.InputJSON, key) {
				t.Fatal("retry did not persist current encrypted connection")
			}
		})
	}
}

func TestAcceptedPlatformTasksKeepOriginalConnection(t *testing.T) {
	s, _, _, _ := creationTestService(t)
	s.SetPlatformChannels([]string{"company-relay"})
	if err := s.SaveLocalModelConfig([]byte(`{"channels":[{"id":"company-relay","baseUrl":"http://127.0.0.1:9001/v1","apiKey":""}]}`)); err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	for _, task := range []model.Task{{ProviderRequestID: "accepted"}, {PollStage: "query"}, {NextPollAt: &now}, {Stage: "submission_unknown"}} {
		task.InputJSON = `{"config":{"platformChannelId":"company-relay","apiKey":"SYNTHETIC_ORIGINAL","baseUrl":"https://old.invalid"}}`
		updated, err := s.preparePlatformTaskCredentials(task)
		if err != nil || updated != "" {
			t.Fatalf("accepted/uncertain task connection changed: %v", err)
		}
	}
	task := model.Task{Type: "canvas_text", InputJSON: `{"config":{"platformChannelId":"company-relay","apiKey":"SYNTHETIC_ORIGINAL"}}`}
	_, _, err := s.processTask(context.Background(), task)
	if err == nil || !strings.Contains(err.Error(), "管理员还没有为你绑定") {
		t.Fatalf("queued task ignored cleared binding: %v", err)
	}
}

func TestQueuedPlatformTaskRefreshesBeforeExecution(t *testing.T) {
	s, db, _, _ := creationTestService(t)
	s.SetPlatformChannels([]string{"company-relay"})
	if err := s.SaveLocalModelConfig([]byte(`{"channels":[{"id":"company-relay","baseUrl":"http://127.0.0.1:9001/v1","apiKey":"SYNTHETIC_NEW"}]}`)); err != nil {
		t.Fatal(err)
	}
	input := map[string]any{"mode": "text", "config": map[string]any{"platformChannelId": "company-relay", "apiKey": "SYNTHETIC_OLD", "baseUrl": "http://127.0.0.1:9000/v1", "model": "text-test", "interfaceType": "chat-completion"}}
	if err := s.protectTaskSecrets(input); err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(input)
	task := model.Task{ID: "queued-current", UserID: "user", Type: "canvas_text", Status: model.TaskStatusRunning, InputJSON: string(raw)}
	if err := db.Create(&task).Error; err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, _, _ = s.processTask(ctx, task)
	stored, err := s.repo.Task(task.ID)
	if err != nil {
		t.Fatal(err)
	}
	plain, err := s.decryptTaskInputJSON(stored.InputJSON)
	if err != nil || !strings.Contains(plain, "SYNTHETIC_NEW") || strings.Contains(plain, "SYNTHETIC_OLD") || strings.Contains(stored.InputJSON, "SYNTHETIC_NEW") {
		t.Fatal("queued task retained creation-time credential")
	}
}

func TestWorkflowFingerprintIncludesReferenceConnection(t *testing.T) {
	before := map[string]any{"config": map[string]any{"referenceAssetService": "https://old.invalid", "referenceAssetOrigin": "old"}}
	after := map[string]any{"config": map[string]any{"referenceAssetService": "https://new.invalid", "referenceAssetOrigin": "old"}}
	if canvasWorkflowCredentialFingerprint(before) == canvasWorkflowCredentialFingerprint(after) {
		t.Fatal("reference service change was ignored")
	}
}

func TestWorkflowKeepsPlatformIdentityAfterAddressChange(t *testing.T) {
	s, _, _, _ := creationTestService(t)
	s.SetPlatformChannels([]string{"company-relay"})
	input := map[string]any{"config": map[string]any{"platformChannelId": "company-relay", "baseUrl": "https://old.invalid", "model": "text-test"}}
	id, err := s.canvasWorkflowChannelID(input, "")
	if err != nil || id != "company-relay" {
		t.Fatalf("persisted platform identity was lost: %s %v", id, err)
	}
}

// An image with ambiguous submission records belongs to its recovery flow:
// preparing credentials must neither block it nor swap its connection.
func TestImageWithSubmissionRecordsKeepsItsRecoveryPath(t *testing.T) {
	s, db, _, _ := creationTestService(t)
	s.SetPlatformChannels([]string{"company-relay"})
	if err := s.SaveLocalModelConfig([]byte(`{"channels":[{"id":"company-relay","baseUrl":"http://127.0.0.1:9001/v1","apiKey":"SYNTHETIC_NEW"}]}`)); err != nil {
		t.Fatal(err)
	}
	input := map[string]any{"mode": "image", "config": map[string]any{"platformChannelId": "company-relay", "apiKey": "SYNTHETIC_OLD", "baseUrl": "http://127.0.0.1:9000/v1", "model": "image-test"}}
	if err := s.protectTaskSecrets(input); err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(input)
	task := model.Task{ID: "image-recovering", UserID: "user", Type: "canvas_image", Status: model.TaskStatusRunning, InputJSON: string(raw)}
	if err := db.Create(&task).Error; err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		if err := db.Create(&model.ApiCallLog{ID: fmt.Sprintf("receipt-%d", i), UserID: "user", TaskID: task.ID, Capability: "image", RequestKind: "create", Method: "POST"}).Error; err != nil {
			t.Fatal(err)
		}
	}
	refreshed, err := s.preparePlatformTaskCredentials(task)
	if err != nil || refreshed != "" {
		t.Fatalf("recovering image was blocked or re-keyed: %q %v", refreshed, err)
	}
}

// Execution refreshed the stored credentials; the executor's copy must pick
// them up so delivery and failure reports use what the request carried.
func TestExecutorPicksUpRefreshedTaskInput(t *testing.T) {
	s, db, _, _ := creationTestService(t)
	task := model.Task{ID: "refreshed-input", UserID: "user", Type: "canvas_image", Status: model.TaskStatusRunning, RouteID: "route", InputJSON: `{"config":{"apiKey":"OLD"}}`}
	if err := db.Create(&task).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Model(&model.Task{}).Where("id = ?", task.ID).Update("input_json", `{"config":{"apiKey":"NEW"}}`).Error; err != nil {
		t.Fatal(err)
	}
	if err := s.refreshTaskProviderState(&task); err != nil {
		t.Fatal(err)
	}
	if task.InputJSON != `{"config":{"apiKey":"NEW"}}` {
		t.Fatalf("executor kept the old input: %s", task.InputJSON)
	}
}

// An image the upstream clearly rejected (401 after a key change) created
// nothing upstream, so a new attempt takes the current key.
func TestRejectedImageTakesTheCurrentKey(t *testing.T) {
	s, db, _, _ := creationTestService(t)
	s.SetPlatformChannels([]string{"company-relay"})
	if err := s.SaveLocalModelConfig([]byte(`{"channels":[{"id":"company-relay","baseUrl":"http://127.0.0.1:9001/v1","apiKey":"SYNTHETIC_NEW"}]}`)); err != nil {
		t.Fatal(err)
	}
	input := map[string]any{"mode": "image", "config": map[string]any{"platformChannelId": "company-relay", "apiKey": "SYNTHETIC_OLD", "baseUrl": "http://127.0.0.1:9001/v1", "model": "image-test"}}
	if err := s.protectTaskSecrets(input); err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(input)
	task := model.Task{ID: "image-rejected", UserID: "user", Type: "canvas_image", Status: model.TaskStatusFailed, InputJSON: string(raw)}
	if err := db.Create(&task).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&model.ApiCallLog{ID: "rejected-receipt", UserID: "user", TaskID: task.ID, Capability: "image", RequestKind: "create", Method: "POST", StatusCode: 401}).Error; err != nil {
		t.Fatal(err)
	}
	refreshed, err := s.preparePlatformTaskCredentials(task)
	if err != nil || refreshed == "" {
		t.Fatalf("rejected image kept the old key: %q %v", refreshed, err)
	}
	plain, err := s.decryptTaskInputJSON(refreshed)
	if err != nil || !strings.Contains(plain, "SYNTHETIC_NEW") {
		t.Fatal("refreshed input lacks the current key")
	}
}
