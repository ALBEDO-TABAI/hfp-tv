package app

import (
	"log"
	"strings"
	"time"

	"infinite-canvas/backend/internal/kernel"
	"infinite-canvas/backend/internal/model"
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
	return report
}
