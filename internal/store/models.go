package store

import "time"

type Status string

const (
	StatusQueued    Status = "QUEUED"
	StatusBuilding  Status = "BUILDING"
	StatusUploading Status = "UPLOADING"
	StatusReady     Status = "READY"
	StatusFailed    Status = "FAILED"
	StatusCanceled  Status = "CANCELED"
)

func (s Status) Terminal() bool {
	return s == StatusReady || s == StatusFailed || s == StatusCanceled
}

type User struct {
	ID        string    `json:"id"`
	GitHubID  *int64    `json:"githubId,omitempty"`
	Login     string    `json:"login"`
	Email     string    `json:"email,omitempty"`
	AvatarURL string    `json:"avatarUrl,omitempty"`
	CreatedAt time.Time `json:"createdAt"`
}

type Project struct {
	ID            string    `json:"id"`
	OwnerID       string    `json:"ownerId"`
	Name          string    `json:"name"`
	Slug          string    `json:"slug"`
	RepoURL       string    `json:"repoUrl"`
	Branch        string    `json:"branch"`
	Framework     string    `json:"framework"`
	InstallCmd    string    `json:"installCommand"`
	BuildCmd      string    `json:"buildCommand"`
	OutputDir     string    `json:"outputDirectory"`
	RootDir       string    `json:"rootDirectory"`
	WebhookSecret string    `json:"-"`
	PublicEnabled bool      `json:"publicEnabled"`
	PublicURL     string    `json:"publicUrl"`
	PublicStatus  string    `json:"publicStatus"`
	CreatedAt     time.Time `json:"createdAt"`
	UpdatedAt     time.Time `json:"updatedAt"`
}

type Deployment struct {
	ID            string     `json:"id"`
	ProjectID     string     `json:"projectId"`
	Source        string     `json:"source"`
	Trigger       string     `json:"trigger"`
	Branch        string     `json:"branch"`
	CommitSHA     string     `json:"commitSha"`
	CommitMessage string     `json:"commitMessage"`
	Status        Status     `json:"status"`
	Error         string     `json:"error"`
	Attempt       int        `json:"attempt"`
	FileCount     int        `json:"fileCount"`
	SizeBytes     int64      `json:"sizeBytes"`
	InstallCmd    string     `json:"installCommand"`
	BuildCmd      string     `json:"buildCommand"`
	OutputDir     string     `json:"outputDirectory"`
	RootDir       string     `json:"rootDirectory"`
	Framework     string     `json:"framework"`
	NodeVersion   string     `json:"nodeVersion"`
	CreatedAt     time.Time  `json:"createdAt"`
	StartedAt     *time.Time `json:"startedAt"`
	FinishedAt    *time.Time `json:"finishedAt"`
}

type Alias struct {
	Hostname     string    `json:"hostname"`
	ProjectID    string    `json:"projectId"`
	DeploymentID string    `json:"deploymentId"`
	UpdatedAt    time.Time `json:"updatedAt"`
}

type EnvVar struct {
	ID         string    `json:"id"`
	ProjectID  string    `json:"projectId"`
	Key        string    `json:"key"`
	Ciphertext []byte    `json:"-"`
	Nonce      []byte    `json:"-"`
	CreatedAt  time.Time `json:"createdAt"`
	UpdatedAt  time.Time `json:"updatedAt"`
}

type LogLine struct {
	Seq    int       `json:"seq"`
	TS     time.Time `json:"ts"`
	Stream string    `json:"stream"`
	Line   string    `json:"line"`
}
