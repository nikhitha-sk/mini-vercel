// Package builder runs untrusted build commands inside short-lived,
// locked-down Docker containers.
//
// Sandbox properties (per build):
//   - fresh container from node:20-alpine, removed afterwards even on failure
//   - runs as the unprivileged "node" user, all capabilities dropped,
//     no-new-privileges, tini as PID 1
//   - CPU, memory (no swap), PID and wall-clock limits
//   - the Docker socket is never mounted; source code is copied in via the
//     Docker API and build output copied out the same way
package builder

import (
	"archive/tar"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/api/types/image"
	"github.com/docker/docker/client"
	"github.com/docker/docker/pkg/stdcopy"

	"github.com/nikhitha-sk/mini-vercel/internal/logstream"
)

const (
	homeDir = "/home/node"
	appDir  = homeDir + "/app"
)

// UserError is a failure caused by the user's code or config (bad build
// command, timeout, missing output). These are never retried.
type UserError struct{ Msg string }

func (e *UserError) Error() string { return e.Msg }

func userErr(format string, a ...any) error { return &UserError{Msg: fmt.Sprintf(format, a...)} }

func IsUserError(err error) bool {
	var ue *UserError
	return errors.As(err, &ue)
}

type Limits struct {
	CPUs     float64
	MemoryMB int64
	PIDs     int64
	Timeout  time.Duration
}

type Sandbox struct {
	docker *client.Client
	image  string
}

func NewSandbox(image string) (*Sandbox, error) {
	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		return nil, err
	}
	return &Sandbox{docker: cli, image: image}, nil
}

func (s *Sandbox) Ping(ctx context.Context) error {
	_, err := s.docker.Ping(ctx)
	return err
}

// EnsureImage pulls the build image if it is not present locally.
func (s *Sandbox) EnsureImage(ctx context.Context, img string) error {
	if _, err := s.docker.ImageInspect(ctx, img); err == nil {
		return nil
	}
	rc, err := s.docker.ImagePull(ctx, img, image.PullOptions{})
	if err != nil {
		return fmt.Errorf("pull %s: %w", img, err)
	}
	defer rc.Close()
	_, err = io.Copy(io.Discard, rc)
	return err
}

// Container is one running build sandbox.
type Container struct {
	sb   *Sandbox
	ID   string
	env  []string
	work string
}

type CreateOptions struct {
	DeploymentID string
	Image        string
	Env          []string
	Limits       Limits
	RootDir      string
}

// Create starts a fresh sandbox container. The caller must always call
// Remove (use defer), so the container never outlives the job.
func (s *Sandbox) Create(ctx context.Context, o CreateOptions) (*Container, error) {
	img := o.Image
	if img == "" {
		img = s.image
	}
	if err := s.EnsureImage(ctx, img); err != nil {
		return nil, err
	}
	name := "mv-build-" + strings.ReplaceAll(o.DeploymentID, "_", "-")
	// A previous attempt (e.g. a worker that crashed mid-build) may have left
	// a container behind; remove it so the retry starts clean.
	_ = s.docker.ContainerRemove(ctx, name, container.RemoveOptions{Force: true})

	env := append([]string{
		"HOME=" + homeDir,
		// ~/.local/bin holds pnpm/yarn shims that run through corepack.
		"PATH=" + homeDir + "/.local/bin:/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin",
		"COREPACK_ENABLE_DOWNLOAD_PROMPT=0",
		"COREPACK_ENABLE_STRICT=0",
		"NPM_CONFIG_UPDATE_NOTIFIER=false",
		"NPM_CONFIG_FUND=false",
		"NPM_CONFIG_AUDIT=false",
		"NPM_CONFIG_PROGRESS=false",
		"NEXT_TELEMETRY_DISABLED=1",
		"ASTRO_TELEMETRY_DISABLED=1",
		"FORCE_COLOR=0",
		"MINI_VERCEL=1",
		"MINI_VERCEL_DEPLOYMENT_ID=" + o.DeploymentID,
	}, o.Env...)

	init := true
	pids := o.Limits.PIDs
	mem := o.Limits.MemoryMB * 1024 * 1024
	resp, err := s.docker.ContainerCreate(ctx,
		&container.Config{
			Image:  img,
			Cmd:    []string{"tail", "-f", "/dev/null"},
			User:   "node",
			Env:    env,
			Labels: map[string]string{"io.minivercel.role": "build", "io.minivercel.deployment": o.DeploymentID},
		},
		&container.HostConfig{
			Init:        &init,
			CapDrop:     []string{"ALL"},
			SecurityOpt: []string{"no-new-privileges:true"},
			Resources: container.Resources{
				NanoCPUs:   int64(o.Limits.CPUs * 1e9),
				Memory:     mem,
				MemorySwap: mem, // no swap
				PidsLimit:  &pids,
			},
			LogConfig: container.LogConfig{Type: "none"},
		}, nil, nil, name)
	if err != nil {
		return nil, fmt.Errorf("create container: %w", err)
	}
	work := appDir
	if o.RootDir != "" {
		work = path.Join(appDir, path.Clean("/"+o.RootDir))
	}
	return &Container{sb: s, ID: resp.ID, env: env, work: work}, nil
}

// CopySource copies a host directory into /home/node/app, owned by "node".
func (c *Container) CopySource(ctx context.Context, dir string) error {
	pr, pw := io.Pipe()
	go func() { pw.CloseWithError(tarDir(dir, "app", pw)) }()
	err := c.sb.docker.CopyToContainer(ctx, c.ID, homeDir, pr, container.CopyToContainerOptions{CopyUIDGID: true})
	pr.Close()
	if err != nil {
		return fmt.Errorf("copy source into container: %w", err)
	}
	return nil
}

func (c *Container) Start(ctx context.Context) error {
	if err := c.sb.docker.ContainerStart(ctx, c.ID, container.StartOptions{}); err != nil {
		return fmt.Errorf("start container: %w", err)
	}
	// Package-manager shims: pnpm and (berry) yarn via corepack, which honours
	// the "packageManager" field without needing root.
	const shims = `mkdir -p ~/.local/bin && ` +
		`printf '#!/bin/sh\nexec corepack pnpm "$@"\n' > ~/.local/bin/pnpm && ` +
		`printf '#!/bin/sh\nexec corepack pnpx "$@"\n' > ~/.local/bin/pnpx && ` +
		`printf '#!/bin/sh\nexec corepack yarn "$@"\n' > ~/.local/bin/yarn && ` +
		`chmod +x ~/.local/bin/*`
	if _, code, err := c.ExecCapture(ctx, shims, homeDir); err != nil || code != 0 {
		return fmt.Errorf("prepare sandbox (exit %d): %v", code, err)
	}
	return nil
}

// WorkDirExists reports whether the configured root directory exists.
func (c *Container) WorkDirExists(ctx context.Context) bool {
	_, err := c.sb.docker.ContainerStatPath(ctx, c.ID, c.work)
	return err == nil
}

// UseRoot resets the working directory to the top of the source tree.
func (c *Container) UseRoot() { c.work = appDir }

// ExecCapture runs a command and returns its combined output and exit code.
func (c *Container) ExecCapture(ctx context.Context, cmd, workDir string) (string, int, error) {
	if workDir == "" {
		workDir = c.work
	}
	ex, err := c.sb.docker.ContainerExecCreate(ctx, c.ID, container.ExecOptions{
		Cmd: []string{"sh", "-c", cmd}, User: "node", WorkingDir: workDir, Env: c.env,
		AttachStdout: true, AttachStderr: true,
	})
	if err != nil {
		return "", -1, err
	}
	att, err := c.sb.docker.ContainerExecAttach(ctx, ex.ID, container.ExecAttachOptions{})
	if err != nil {
		return "", -1, err
	}
	defer att.Close()
	var out strings.Builder
	if _, err := stdcopy.StdCopy(&out, &out, att.Reader); err != nil {
		return "", -1, err
	}
	insp, err := c.sb.docker.ContainerExecInspect(ctx, ex.ID)
	if err != nil {
		return "", -1, err
	}
	return out.String(), insp.ExitCode, nil
}

// FindOutput returns the first candidate directory (relative to the working
// directory) that contains an index.html, or failing that any files.
// Candidates may contain shell globs such as dist/*/browser.
func (c *Container) FindOutput(ctx context.Context, candidates []string) (string, bool) {
	var parts []string
	for _, cand := range candidates {
		if strings.ContainsAny(cand, "'\"$`\\;&|<>(){}\n") {
			continue
		}
		if strings.Contains(cand, "*") {
			parts = append(parts, cand)
		} else {
			parts = append(parts, "'"+cand+"'")
		}
	}
	if len(parts) == 0 {
		return "", false
	}
	list := strings.Join(parts, " ")
	script := `for d in ` + list + `; do if [ -f "$d/index.html" ]; then echo "$d"; exit 0; fi; done; ` +
		`for d in ` + list + `; do if [ "$d" != "." ] && [ -d "$d" ] && [ -n "$(ls -A "$d" 2>/dev/null)" ]; then echo "$d"; exit 0; fi; done; exit 1`
	out, code, err := c.ExecCapture(ctx, script, "")
	if err != nil || code != 0 {
		return "", false
	}
	return strings.TrimSpace(out), true
}

// Exec runs one shell command in the sandbox, streaming output to the logger.
// Non-zero exits, timeouts and OOM kills are returned as *UserError.
func (c *Container) Exec(ctx context.Context, cmd string, l *logstream.Logger) error {
	ex, err := c.sb.docker.ContainerExecCreate(ctx, c.ID, container.ExecOptions{
		Cmd:          []string{"sh", "-c", cmd},
		User:         "node",
		WorkingDir:   c.work,
		Env:          c.env,
		AttachStdout: true,
		AttachStderr: true,
	})
	if err != nil {
		return fmt.Errorf("exec create: %w", err)
	}
	att, err := c.sb.docker.ContainerExecAttach(ctx, ex.ID, container.ExecAttachOptions{})
	if err != nil {
		return fmt.Errorf("exec attach: %w", err)
	}
	defer att.Close()

	stdout := &logstream.LineWriter{L: l, Stream: "stdout"}
	stderr := &logstream.LineWriter{L: l, Stream: "stderr"}
	copyDone := make(chan error, 1)
	go func() {
		_, err := stdcopy.StdCopy(stdout, stderr, att.Reader)
		copyDone <- err
	}()

	select {
	case <-ctx.Done():
		att.Close()
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return userErr("Build exceeded the time limit and was stopped")
		}
		return ctx.Err()
	case <-copyDone:
	}
	stdout.Flush()
	stderr.Flush()

	insp, err := c.sb.docker.ContainerExecInspect(ctx, ex.ID)
	if err != nil {
		return fmt.Errorf("exec inspect: %w", err)
	}
	switch insp.ExitCode {
	case 0:
		return nil
	case 137:
		return userErr("Command %q was killed (exit 137). The build likely ran out of memory", cmd)
	default:
		return userErr("Command %q exited with code %d", cmd, insp.ExitCode)
	}
}

// WalkOutput streams every regular file under the output directory to fn,
// with paths relative to the output directory.
func (c *Container) WalkOutput(ctx context.Context, outputDir string, fn func(rel string, size int64, r io.Reader) error) error {
	src := path.Join(c.work, path.Clean("/"+outputDir))
	rc, _, err := c.sb.docker.CopyFromContainer(ctx, c.ID, src)
	if err != nil {
		if client.IsErrNotFound(err) || strings.Contains(err.Error(), "No such container:path") || strings.Contains(err.Error(), "Could not find the file") {
			return userErr("Output directory %q was not found after the build. Check the Output Directory setting", outputDir)
		}
		return fmt.Errorf("copy output: %w", err)
	}
	defer rc.Close()
	tr := tar.NewReader(rc)
	found := 0
	for {
		h, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("read output tar: %w", err)
		}
		if h.Typeflag != tar.TypeReg {
			continue
		}
		// Strip the leading directory component (the output dir itself).
		rel := h.Name
		if i := strings.IndexByte(rel, '/'); i >= 0 {
			rel = rel[i+1:]
		}
		rel = strings.TrimPrefix(path.Clean("/"+rel), "/")
		if rel == "" || strings.HasPrefix(rel, "node_modules/") || strings.HasPrefix(rel, ".git/") {
			continue
		}
		found++
		if err := fn(rel, h.Size, tr); err != nil {
			return err
		}
	}
	if found == 0 {
		return userErr("Output directory %q is empty", outputDir)
	}
	return nil
}

// Remove force-removes the container. It uses its own context so it still
// runs after the job context was canceled or timed out.
func (c *Container) Remove() {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := c.sb.docker.ContainerRemove(ctx, c.ID, container.RemoveOptions{Force: true, RemoveVolumes: true}); err != nil {
		slog.Warn("remove build container", "id", c.ID[:12], "err", err)
	}
}

// Janitor removes leftover build containers older than maxAge (crash
// recovery, see "disk fills up" in the spec's risks section).
func (s *Sandbox) Janitor(ctx context.Context, maxAge time.Duration) {
	list, err := s.docker.ContainerList(ctx, container.ListOptions{All: true, Filters: filters.NewArgs(filters.Arg("label", "io.minivercel.role=build"))})
	if err != nil {
		return
	}
	for _, ct := range list {
		if time.Since(time.Unix(ct.Created, 0)) > maxAge {
			_ = s.docker.ContainerRemove(ctx, ct.ID, container.RemoveOptions{Force: true, RemoveVolumes: true})
		}
	}
}

// tarDir writes dir as a tar stream with entries prefixed by prefix/.
func tarDir(dir, prefix string, w io.Writer) error {
	tw := tar.NewWriter(w)
	err := filepath.WalkDir(dir, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(dir, p)
		if err != nil {
			return err
		}
		name := prefix
		if rel != "." {
			name = prefix + "/" + filepath.ToSlash(rel)
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		var link string
		if info.Mode()&os.ModeSymlink != 0 {
			if link, err = os.Readlink(p); err != nil {
				return err
			}
		}
		h, err := tar.FileInfoHeader(info, link)
		if err != nil {
			return err
		}
		h.Name = name
		if d.IsDir() {
			h.Name += "/"
			h.Mode = 0o755
		} else if h.Mode&0o111 != 0 {
			h.Mode = 0o755
		} else {
			h.Mode = 0o644
		}
		h.Uid, h.Gid, h.Uname, h.Gname = 1000, 1000, "node", "node"
		if err := tw.WriteHeader(h); err != nil {
			return err
		}
		if info.Mode().IsRegular() {
			f, err := os.Open(p)
			if err != nil {
				return err
			}
			_, err = io.Copy(tw, f)
			f.Close()
			return err
		}
		return nil
	})
	if err != nil {
		return err
	}
	return tw.Close()
}
