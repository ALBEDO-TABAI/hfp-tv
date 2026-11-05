package task

import (
	"infinite-canvas/backend/internal/model"
	"testing"
)

func TestRetryRefreshAllowsReferenceServiceButNotContent(t *testing.T) {
	s := NewService(nil, Dependencies{Secrets: passSecrets{}})
	before := `{"prompt":"keep","config":{"apiKey":"SYNTHETIC_OLD","referenceAssetService":"https://old.invalid"}}`
	task := &model.Task{InputJSON: before}
	for _, after := range []string{
		`{"prompt":"keep","config":{"apiKey":"SYNTHETIC_OLD","referenceAssetService":"https://new.invalid"}}`,
		`{"prompt":"keep","config":{"apiKey":"SYNTHETIC_NEW","referenceAssetService":"https://new.invalid"}}`,
	} {
		if err := s.validateRetryCredentialRefresh(task, retryCredentialRefresh{before, after}); err != nil {
			t.Errorf("connection refresh rejected: %v", err)
		}
	}
	after := `{"prompt":"changed","config":{"apiKey":"SYNTHETIC_NEW","referenceAssetService":"https://new.invalid"}}`
	if err := s.validateRetryCredentialRefresh(task, retryCredentialRefresh{before, after}); err == nil {
		t.Fatal("content change allowed")
	}
	task.ProviderRequestID = "accepted"
	if err := s.validateRetryCredentialRefresh(task, retryCredentialRefresh{before, after}); err == nil {
		t.Fatal("accepted task changed")
	}
}
