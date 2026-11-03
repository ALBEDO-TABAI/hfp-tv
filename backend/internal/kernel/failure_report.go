package kernel

import "time"

// FailureReport is a terminal failure a hosted workspace kernel hands to the
// team server's error log. It carries only what the member already sees in
// the task detail (message, stage, task log), never credentials, request
// bodies or result URLs.
type FailureReport struct {
	Source         string // "generation"
	TaskID         string
	TaskType       string
	Model          string
	Stage          string
	Message        string
	Category       string
	ProviderTaskID string
	HTTPStatus     int
	// UpstreamDetail is the diagnostics summary, e.g. the upstream code.
	UpstreamDetail string
	StartedAt      *time.Time
	FailedAt       time.Time
	// Log holds the task's most recent log lines, oldest first.
	Log []string
}
