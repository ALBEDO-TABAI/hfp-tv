package app

import (
	"encoding/json"
	"errors"
	"log"
	"slices"
	"sort"
	"strings"
	"time"

	"infinite-canvas/backend/internal/kernel"
	"infinite-canvas/backend/internal/model"

	"gorm.io/gorm"
)

const failureReportLogLines = 12

// SetFailureReporter receives every task that ends in failure. A team server
// sets it on hosted kernels to keep an error log the administrator can read
// without asking members for screenshots; desktop leaves it nil.
func (s *Service) SetFailureReporter(report func(kernel.FailureReport)) {
	if s == nil {
		return
	}
	s.failureReporter = report
}

// reportTaskFailure runs after the failed state is stored. It never blocks
// the worker: the report, with the task's recent log lines, is built and
// delivered in the background.
func (s *Service) reportTaskFailure(task model.Task) {
	if s == nil || s.failureReporter == nil {
		return
	}
	report := s.failureReporter
	go func() {
		defer func() {
			if recovered := recover(); recovered != nil {
				log.Printf("task failure report panicked: task_id=%s error=%v", task.ID, recovered)
			}
		}()
		report(s.taskFailureReport(task))
	}()
}

func (s *Service) taskFailureReport(task model.Task) kernel.FailureReport {
	failedAt := time.Now()
	if task.CompletedAt != nil {
		failedAt = *task.CompletedAt
	}
	report := kernel.FailureReport{
		Source: "task", TaskID: task.ID, TaskType: task.Type, Model: task.Model, Stage: task.Stage,
		Message: task.Error, Category: persistedFailureErrorCode(task.Error, task.Stage),
		ProviderTaskID: task.ProviderRequestID, StartedAt: task.StartedAt, FailedAt: failedAt,
	}
	if diagnostics := task.FailureDiagnostics; diagnostics != nil {
		report.HTTPStatus = diagnostics.HTTPStatus
		report.ProviderTaskID = firstNonEmpty(report.ProviderTaskID, diagnostics.ProviderTaskID)
		detail := make([]string, 0, 3)
		for _, part := range []string{diagnostics.ProviderCode, diagnostics.Summary, diagnostics.FailedOperation} {
			// The summary often repeats the message the member saw.
			if part = strings.TrimSpace(part); part != "" && !strings.Contains(task.Error, part) {
				detail = append(detail, part)
			}
		}
		report.UpstreamDetail = strings.Join(detail, " · ")
	}
	if s.repo != nil {
		if logs, err := s.repo.TaskLogs(task.UserID, task.ID); err == nil {
			if len(logs) > failureReportLogLines {
				logs = logs[len(logs)-failureReportLogLines:]
			}
			for _, entry := range logs {
				report.Log = append(report.Log, entry.CreatedAt.Format("15:04:05")+" "+strings.ToUpper(entry.Level)+" "+entry.Message)
			}
		}
	}
	// The report leaves this workspace for the administrators' error page. An
	// upstream may echo the request's key in any format, so remove this task's
	// own keys exactly, on top of the pattern-based redaction.
	secrets, readable := s.taskInputSecrets(task)
	if !readable {
		// The task's keys are unknown, so no upstream text can be cleared of
		// them: report only that the task failed, at which stage.
		report.Message, report.UpstreamDetail, report.ProviderTaskID, report.Log = "任务失败（无法确认上游原文不含密钥，未上报详情）", "", "", nil
	}
	if len(secrets) > 0 {
		scrub := func(value string) string {
			for _, secret := range secrets {
				value = strings.ReplaceAll(value, secret, "[REDACTED]")
			}
			return value
		}
		report.Message, report.UpstreamDetail, report.Stage, report.Model = scrub(report.Message), scrub(report.UpstreamDetail), scrub(report.Stage), scrub(report.Model)
		report.ProviderTaskID = scrub(report.ProviderTaskID)
		for i := range report.Log {
			report.Log[i] = scrub(report.Log[i])
		}
	}
	return report
}

// taskInputSecrets lists the credentials a task was sent with, longest first,
// so they can be removed from anything that leaves the workspace.
func (s *Service) taskInputSecrets(task model.Task) ([]string, bool) {
	if s == nil {
		return nil, strings.TrimSpace(task.InputJSON) == ""
	}
	inputs := []string{task.InputJSON}
	// Execution may have refreshed a company relay task's key after this copy
	// was read; the stored input has the key the request actually carried.
	if s.repo != nil && task.ID != "" {
		latest, err := s.repo.Task(task.ID)
		switch {
		case errors.Is(err, gorm.ErrRecordNotFound):
			// No stored row: the copy at hand is all there is.
		case err != nil:
			return nil, false
		case latest.InputJSON != task.InputJSON:
			inputs = append(inputs, latest.InputJSON)
		}
	}
	var secrets []string
	readable := true
	for _, raw := range inputs {
		values, ok := s.inputSecrets(raw)
		readable = readable && ok
		for _, secret := range values {
			if !slices.Contains(secrets, secret) {
				secrets = append(secrets, secret)
			}
		}
	}
	sort.Slice(secrets, func(i, j int) bool { return len(secrets[i]) > len(secrets[j]) })
	return secrets, readable
}

func (s *Service) inputSecrets(raw string) ([]string, bool) {
	if strings.TrimSpace(raw) == "" {
		return nil, true
	}
	plain, err := s.decryptTaskInputJSON(raw)
	if err != nil {
		return nil, false
	}
	var input struct {
		Config struct {
			APIKey                 string `json:"apiKey"`
			SecretKey              string `json:"secretKey"`
			RunningHubUploadAPIKey string `json:"runningHubUploadApiKey"`
			Headers                []struct {
				Value string `json:"value"`
			} `json:"headers"`
		} `json:"config"`
	}
	if json.Unmarshal([]byte(plain), &input) != nil {
		return nil, false
	}
	values := []string{input.Config.APIKey, input.Config.SecretKey, input.Config.RunningHubUploadAPIKey}
	for _, header := range input.Config.Headers {
		value := strings.TrimSpace(header.Value)
		if scheme, token, found := strings.Cut(value, " "); found && strings.EqualFold(scheme, "Bearer") {
			value = token
		}
		values = append(values, value)
	}
	var secrets []string
	for _, value := range values {
		// Very short values would garble ordinary words; real keys are longer.
		if value = strings.TrimSpace(value); len(value) >= 6 && !slices.Contains(secrets, value) {
			secrets = append(secrets, value)
		}
	}
	return secrets, true
}
