package app

import (
	"encoding/json"
	"infinite-canvas/backend/internal/model"
	"strings"
)

// Only a new submission may change connection identity. Accepted or uncertain
// work keeps its original credentials so recovery never becomes regeneration.
func (s *Service) preparePlatformTaskCredentials(task model.Task) (string, error) {
	if !strings.Contains(task.InputJSON, `"platformChannelId"`) {
		return "", nil
	}
	if task.ProviderRequestID != "" || task.PollStage != "" || task.NextPollAt != nil || task.Stage == "submission_unknown" {
		return "", nil
	}
	plain, err := s.decryptTaskInputJSON(task.InputJSON)
	if err != nil {
		return "", err
	}
	var input map[string]any
	if err := json.Unmarshal([]byte(plain), &input); err != nil {
		return "", err
	}
	config, _ := input["config"].(map[string]any)
	if stringValue(config["platformChannelId"]) == "" {
		return "", nil
	}
	if task.Type == "canvas_image" {
		// An accepted or unconfirmed image submission (including several
		// ambiguous ones) belongs to the recovery flow and keeps its original
		// request. An image the upstream clearly rejected (for example 401
		// after a key change) created nothing, so a new attempt may take the
		// current credentials.
		receipt, err := s.repo.ImageCreateReceipt(task.UserID, task.ID)
		if err != nil || (receipt != nil && receipt.StatusCode < 400) {
			return "", nil
		}
	}
	previous := canvasWorkflowCredentialFingerprint(input)
	input, err = s.resolvePlatformChannelSecrets(input)
	if err != nil {
		return "", err
	}
	if previous == canvasWorkflowCredentialFingerprint(input) {
		return "", nil
	}
	if err := s.protectTaskSecrets(input); err != nil {
		return "", err
	}
	encoded, err := json.Marshal(input)
	return string(encoded), err
}
