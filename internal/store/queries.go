package store

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"
)

// ---------- users ----------

// EnsureLocalUser returns the single local user used until GitHub OAuth is
// enabled. It is created on first boot.
func (s *Store) EnsureLocalUser(ctx context.Context, id string) (User, error) {
	_, err := s.DB.Exec(ctx, `INSERT INTO users (id, login) VALUES ($1, 'local') ON CONFLICT (id) DO NOTHING`, id)
	if err != nil {
		return User{}, err
	}
	var u User
	err = s.DB.QueryRow(ctx, `SELECT id, github_id, login, COALESCE(email,''), COALESCE(avatar_url,''), created_at FROM users WHERE id=$1`, id).
		Scan(&u.ID, &u.GitHubID, &u.Login, &u.Email, &u.AvatarURL, &u.CreatedAt)
	return u, err
}

// ---------- projects ----------

const projectCols = `id, owner_id, name, slug, repo_url, branch, framework, install_cmd, build_cmd, output_dir, root_dir, webhook_secret, created_at, updated_at`

func scanProject(r pgx.Row) (Project, error) {
	var p Project
	err := r.Scan(&p.ID, &p.OwnerID, &p.Name, &p.Slug, &p.RepoURL, &p.Branch, &p.Framework, &p.InstallCmd, &p.BuildCmd, &p.OutputDir, &p.RootDir, &p.WebhookSecret, &p.CreatedAt, &p.UpdatedAt)
	return p, notFound(err)
}

func (s *Store) CreateProject(ctx context.Context, p Project) (Project, error) {
	return scanProject(s.DB.QueryRow(ctx, `
		INSERT INTO projects (id, owner_id, name, slug, repo_url, branch, framework, install_cmd, build_cmd, output_dir, root_dir, webhook_secret)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)
		RETURNING `+projectCols,
		p.ID, p.OwnerID, p.Name, p.Slug, p.RepoURL, p.Branch, p.Framework, p.InstallCmd, p.BuildCmd, p.OutputDir, p.RootDir, p.WebhookSecret))
}

func (s *Store) UpdateProject(ctx context.Context, p Project) (Project, error) {
	return scanProject(s.DB.QueryRow(ctx, `
		UPDATE projects SET name=$2, repo_url=$3, branch=$4, framework=$5, install_cmd=$6, build_cmd=$7, output_dir=$8, root_dir=$9, updated_at=now()
		WHERE id=$1 RETURNING `+projectCols,
		p.ID, p.Name, p.RepoURL, p.Branch, p.Framework, p.InstallCmd, p.BuildCmd, p.OutputDir, p.RootDir))
}

func (s *Store) GetProject(ctx context.Context, id string) (Project, error) {
	return scanProject(s.DB.QueryRow(ctx, `SELECT `+projectCols+` FROM projects WHERE id=$1 OR slug=$1`, id))
}

func (s *Store) ListProjects(ctx context.Context, ownerID string) ([]Project, error) {
	rows, err := s.DB.Query(ctx, `SELECT `+projectCols+` FROM projects WHERE owner_id=$1 ORDER BY updated_at DESC`, ownerID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Project{}
	for rows.Next() {
		p, err := scanProject(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

func (s *Store) DeleteProject(ctx context.Context, id string) error {
	_, err := s.DB.Exec(ctx, `DELETE FROM projects WHERE id=$1`, id)
	return err
}

func (s *Store) SlugExists(ctx context.Context, slug string) (bool, error) {
	var ok bool
	err := s.DB.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM projects WHERE slug=$1)`, slug).Scan(&ok)
	return ok, err
}

func (s *Store) TouchProject(ctx context.Context, id string) error {
	_, err := s.DB.Exec(ctx, `UPDATE projects SET updated_at=now() WHERE id=$1`, id)
	return err
}

// ---------- deployments ----------

const deploymentCols = `id, project_id, source, trigger, branch, commit_sha, commit_message, status, error, attempt, file_count, size_bytes, install_cmd, build_cmd, output_dir, root_dir, created_at, started_at, finished_at`

func scanDeployment(r pgx.Row) (Deployment, error) {
	var d Deployment
	err := r.Scan(&d.ID, &d.ProjectID, &d.Source, &d.Trigger, &d.Branch, &d.CommitSHA, &d.CommitMessage, &d.Status, &d.Error, &d.Attempt, &d.FileCount, &d.SizeBytes, &d.InstallCmd, &d.BuildCmd, &d.OutputDir, &d.RootDir, &d.CreatedAt, &d.StartedAt, &d.FinishedAt)
	return d, notFound(err)
}

func (s *Store) CreateDeployment(ctx context.Context, d Deployment) (Deployment, error) {
	return scanDeployment(s.DB.QueryRow(ctx, `
		INSERT INTO deployments (id, project_id, source, trigger, branch, commit_sha, commit_message, status, install_cmd, build_cmd, output_dir, root_dir)
		VALUES ($1,$2,$3,$4,$5,$6,$7,'QUEUED',$8,$9,$10,$11)
		RETURNING `+deploymentCols,
		d.ID, d.ProjectID, d.Source, d.Trigger, d.Branch, d.CommitSHA, d.CommitMessage, d.InstallCmd, d.BuildCmd, d.OutputDir, d.RootDir))
}

func (s *Store) GetDeployment(ctx context.Context, id string) (Deployment, error) {
	return scanDeployment(s.DB.QueryRow(ctx, `SELECT `+deploymentCols+` FROM deployments WHERE id=$1`, id))
}

func (s *Store) ListDeployments(ctx context.Context, projectID string, limit int) ([]Deployment, error) {
	rows, err := s.DB.Query(ctx, `SELECT `+deploymentCols+` FROM deployments WHERE project_id=$1 ORDER BY created_at DESC LIMIT $2`, projectID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Deployment{}
	for rows.Next() {
		d, err := scanDeployment(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

func (s *Store) ListRecentDeployments(ctx context.Context, ownerID string, limit int) ([]Deployment, error) {
	rows, err := s.DB.Query(ctx, `SELECT `+prefixed("d", deploymentCols)+` FROM deployments d JOIN projects p ON p.id=d.project_id WHERE p.owner_id=$1 ORDER BY d.created_at DESC LIMIT $2`, ownerID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Deployment{}
	for rows.Next() {
		d, err := scanDeployment(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

// MarkBuilding moves a deployment into BUILDING (only from a non-terminal
// state) and bumps the attempt counter. ok=false means the deployment was
// already READY/FAILED/CANCELED and the job must not run.
func (s *Store) MarkBuilding(ctx context.Context, id string) (Deployment, bool, error) {
	d, err := scanDeployment(s.DB.QueryRow(ctx, `
		UPDATE deployments SET status='BUILDING', attempt=attempt+1, error='', started_at=COALESCE(started_at, now())
		WHERE id=$1 AND status IN ('QUEUED','BUILDING','UPLOADING')
		RETURNING `+deploymentCols, id))
	if err == ErrNotFound {
		return Deployment{}, false, nil
	}
	return d, err == nil, err
}

func (s *Store) SetStatus(ctx context.Context, id string, st Status) error {
	_, err := s.DB.Exec(ctx, `UPDATE deployments SET status=$2 WHERE id=$1 AND status NOT IN ('READY','FAILED','CANCELED')`, id, st)
	return err
}

func (s *Store) SetCommit(ctx context.Context, id, sha, msg string) error {
	_, err := s.DB.Exec(ctx, `UPDATE deployments SET commit_sha=$2, commit_message=$3 WHERE id=$1`, id, sha, msg)
	return err
}

// Finish moves a deployment to a terminal state. It never overwrites an
// existing terminal state (e.g. a user cancel that raced the build).
func (s *Store) Finish(ctx context.Context, id string, st Status, errMsg string, files int, size int64) (bool, error) {
	tag, err := s.DB.Exec(ctx, `
		UPDATE deployments SET status=$2, error=$3, file_count=$4, size_bytes=$5, finished_at=now()
		WHERE id=$1 AND status NOT IN ('READY','FAILED','CANCELED')`, id, st, errMsg, files, size)
	if err != nil {
		return false, err
	}
	return tag.RowsAffected() == 1, nil
}

func (s *Store) Cancel(ctx context.Context, id string) (bool, error) {
	tag, err := s.DB.Exec(ctx, `
		UPDATE deployments SET status='CANCELED', error='Canceled by user', finished_at=now()
		WHERE id=$1 AND status IN ('QUEUED','BUILDING','UPLOADING')`, id)
	if err != nil {
		return false, err
	}
	return tag.RowsAffected() == 1, nil
}

// ---------- aliases ----------

// Promote points hostname at a READY deployment. One UPSERT = instant switch.
func (s *Store) Promote(ctx context.Context, hostname, projectID, deploymentID string) error {
	_, err := s.DB.Exec(ctx, `
		INSERT INTO aliases (hostname, project_id, deployment_id, updated_at) VALUES ($1,$2,$3,now())
		ON CONFLICT (hostname) DO UPDATE SET deployment_id=EXCLUDED.deployment_id, updated_at=now()`,
		hostname, projectID, deploymentID)
	return err
}

func (s *Store) ProjectAliases(ctx context.Context, projectID string) ([]Alias, error) {
	rows, err := s.DB.Query(ctx, `SELECT hostname, project_id, deployment_id, updated_at FROM aliases WHERE project_id=$1 ORDER BY hostname`, projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Alias{}
	for rows.Next() {
		var a Alias
		if err := rows.Scan(&a.Hostname, &a.ProjectID, &a.DeploymentID, &a.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

// ResolveHost returns the deployment an alias hostname points at.
func (s *Store) ResolveHost(ctx context.Context, hostname string) (string, error) {
	var id string
	err := s.DB.QueryRow(ctx, `SELECT deployment_id FROM aliases WHERE hostname=$1`, hostname).Scan(&id)
	return id, notFound(err)
}

func (s *Store) DeploymentStatus(ctx context.Context, id string) (Status, error) {
	var st Status
	err := s.DB.QueryRow(ctx, `SELECT status FROM deployments WHERE id=$1`, id).Scan(&st)
	return st, notFound(err)
}

// ---------- env vars ----------

func (s *Store) ListEnvVars(ctx context.Context, projectID string) ([]EnvVar, error) {
	rows, err := s.DB.Query(ctx, `SELECT id, project_id, key, ciphertext, nonce, created_at, updated_at FROM env_vars WHERE project_id=$1 ORDER BY key`, projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []EnvVar{}
	for rows.Next() {
		var e EnvVar
		if err := rows.Scan(&e.ID, &e.ProjectID, &e.Key, &e.Ciphertext, &e.Nonce, &e.CreatedAt, &e.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

func (s *Store) UpsertEnvVar(ctx context.Context, e EnvVar) error {
	_, err := s.DB.Exec(ctx, `
		INSERT INTO env_vars (id, project_id, key, ciphertext, nonce) VALUES ($1,$2,$3,$4,$5)
		ON CONFLICT (project_id, key) DO UPDATE SET ciphertext=EXCLUDED.ciphertext, nonce=EXCLUDED.nonce, updated_at=now()`,
		e.ID, e.ProjectID, e.Key, e.Ciphertext, e.Nonce)
	return err
}

func (s *Store) DeleteEnvVar(ctx context.Context, projectID, key string) error {
	_, err := s.DB.Exec(ctx, `DELETE FROM env_vars WHERE project_id=$1 AND key=$2`, projectID, key)
	return err
}

// ---------- build logs ----------

func (s *Store) NextLogSeq(ctx context.Context, deploymentID string) (int, error) {
	var n int
	err := s.DB.QueryRow(ctx, `SELECT COALESCE(MAX(seq), 0) FROM build_logs WHERE deployment_id=$1`, deploymentID).Scan(&n)
	return n + 1, err
}

func (s *Store) InsertLogs(ctx context.Context, deploymentID string, lines []LogLine) error {
	if len(lines) == 0 {
		return nil
	}
	_, err := s.DB.CopyFrom(ctx, pgx.Identifier{"build_logs"}, []string{"deployment_id", "seq", "ts", "stream", "line"},
		pgx.CopyFromSlice(len(lines), func(i int) ([]any, error) {
			l := lines[i]
			return []any{deploymentID, l.Seq, l.TS, l.Stream, l.Line}, nil
		}))
	return err
}

func (s *Store) ListLogs(ctx context.Context, deploymentID string, afterSeq int) ([]LogLine, error) {
	rows, err := s.DB.Query(ctx, `SELECT seq, ts, stream, line FROM build_logs WHERE deployment_id=$1 AND seq>$2 ORDER BY seq LIMIT 20000`, deploymentID, afterSeq)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []LogLine{}
	for rows.Next() {
		var l LogLine
		if err := rows.Scan(&l.Seq, &l.TS, &l.Stream, &l.Line); err != nil {
			return nil, err
		}
		out = append(out, l)
	}
	return out, rows.Err()
}

// ---------- stats ----------

type Stats struct {
	Projects        int     `json:"projects"`
	Deployments     int     `json:"deployments"`
	Ready           int     `json:"ready"`
	Failed          int     `json:"failed"`
	Building        int     `json:"building"`
	AvgBuildSeconds float64 `json:"avgBuildSeconds"`
	TotalBytes      int64   `json:"totalBytes"`
}

func (s *Store) Stats(ctx context.Context, ownerID string, since time.Time) (Stats, error) {
	var st Stats
	err := s.DB.QueryRow(ctx, `
		SELECT
		  (SELECT count(*) FROM projects WHERE owner_id=$1),
		  count(d.*),
		  count(*) FILTER (WHERE d.status='READY'),
		  count(*) FILTER (WHERE d.status='FAILED'),
		  count(*) FILTER (WHERE d.status IN ('QUEUED','BUILDING','UPLOADING')),
		  COALESCE(avg(EXTRACT(EPOCH FROM (d.finished_at - d.started_at))) FILTER (WHERE d.status='READY'), 0),
		  COALESCE(sum(d.size_bytes) FILTER (WHERE d.status='READY'), 0)
		FROM deployments d JOIN projects p ON p.id=d.project_id
		WHERE p.owner_id=$1 AND d.created_at >= $2`, ownerID, since).
		Scan(&st.Projects, &st.Deployments, &st.Ready, &st.Failed, &st.Building, &st.AvgBuildSeconds, &st.TotalBytes)
	return st, err
}

func prefixed(alias, cols string) string {
	out := ""
	start := 0
	for i := 0; i <= len(cols); i++ {
		if i == len(cols) || cols[i] == ',' {
			col := cols[start:i]
			for len(col) > 0 && col[0] == ' ' {
				col = col[1:]
			}
			if out != "" {
				out += ", "
			}
			out += alias + "." + col
			start = i + 1
		}
	}
	return out
}
