// Package queue defines the asynq build job shared by the API (producer)
// and the build workers (consumers).
package queue

import (
	"encoding/json"
	"time"

	"github.com/hibiken/asynq"
)

const (
	TypeBuild  = "deployment:build"
	QueueBuild = "builds"
	MaxRetry   = 3
)

type Source struct {
	Type   string `json:"type"` // "git" | "zip"
	Repo   string `json:"repo,omitempty"`
	Branch string `json:"branch,omitempty"`
	Commit string `json:"commit,omitempty"`
	ZipKey string `json:"zipKey,omitempty"`
}

type Build struct {
	Image     string `json:"image"`
	Install   string `json:"install"`
	Command   string `json:"command"`
	OutputDir string `json:"outputDir"`
	RootDir   string `json:"rootDir,omitempty"`
}

type Limits struct {
	CPUs       float64 `json:"cpus"`
	MemoryMB   int64   `json:"memoryMB"`
	PIDs       int64   `json:"pids"`
	TimeoutSec int     `json:"timeoutSec"`
}

// BuildPayload mirrors the job payload in the spec.
type BuildPayload struct {
	DeploymentID string `json:"deploymentId"`
	ProjectID    string `json:"projectId"`
	Source       Source `json:"source"`
	Build        Build  `json:"build"`
	Limits       Limits `json:"limits"`
}

func NewBuildTask(p BuildPayload) (*asynq.Task, error) {
	b, err := json.Marshal(p)
	if err != nil {
		return nil, err
	}
	timeout := time.Duration(p.Limits.TimeoutSec)*time.Second + 5*time.Minute // headroom for clone + upload
	return asynq.NewTask(TypeBuild, b,
		asynq.Queue(QueueBuild),
		asynq.MaxRetry(MaxRetry),
		asynq.Timeout(timeout),
		asynq.TaskID(p.DeploymentID), // dedupe: one job per deployment
		asynq.Retention(24*time.Hour),
	), nil
}

// RetryDelay is exponential backoff: 5s, 10s, 20s, ...
func RetryDelay(n int, _ error, _ *asynq.Task) time.Duration {
	d := 5 * time.Second
	for i := 0; i < n; i++ {
		d *= 2
	}
	if d > 2*time.Minute {
		d = 2 * time.Minute
	}
	return d
}
