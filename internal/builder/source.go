package builder

import (
	"archive/zip"
	"bytes"
	"context"
	"fmt"
	"io"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/nikhitha-sk/mini-vercel/internal/logstream"
)

// ValidateRepoURL accepts public https Git URLs only (no file://, ssh or
// local paths, which would let a user read the worker's filesystem).
func ValidateRepoURL(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", fmt.Errorf("repository URL is required")
	}
	if !strings.Contains(raw, "://") {
		raw = "https://" + raw
	}
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "https" || u.Host == "" || u.User != nil {
		return "", fmt.Errorf("repository URL must be a public https URL, e.g. https://github.com/user/repo")
	}
	if strings.Count(strings.Trim(u.Path, "/"), "/") < 1 {
		return "", fmt.Errorf("repository URL must include the owner and repository name")
	}
	u.RawQuery, u.Fragment = "", ""
	return strings.TrimSuffix(u.String(), "/"), nil
}

type Commit struct{ SHA, Message string }

// Clone performs a shallow clone of branch into dir and returns the HEAD commit.
func Clone(ctx context.Context, repo, branch, dir string, l *logstream.Logger) (Commit, error) {
	if _, err := ValidateRepoURL(repo); err != nil {
		return Commit{}, &UserError{Msg: err.Error()}
	}
	args := []string{"clone", "--depth", "1", "--single-branch", "--no-tags"}
	if branch != "" {
		args = append(args, "--branch", branch)
	}
	args = append(args, "--", repo, dir)
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0", "GIT_LFS_SKIP_SMUDGE=1")
	var out bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &out
	if err := cmd.Run(); err != nil {
		if ctx.Err() != nil {
			return Commit{}, ctx.Err()
		}
		for _, line := range strings.Split(strings.TrimSpace(out.String()), "\n") {
			if line != "" {
				l.Line("stderr", line)
			}
		}
		msg := out.String()
		// Network / DNS problems are infrastructure errors (retry); bad repo
		// or branch names are the user's (fail fast).
		if strings.Contains(msg, "Could not resolve host") || strings.Contains(msg, "Connection timed out") || strings.Contains(msg, "Connection reset") {
			return Commit{}, fmt.Errorf("git clone: %s", strings.TrimSpace(msg))
		}
		return Commit{}, userErr("Could not clone %s (branch %q). Is the repository public and the branch name correct?", repo, branch)
	}
	sha, _ := gitOut(ctx, dir, "rev-parse", "HEAD")
	msg, _ := gitOut(ctx, dir, "log", "-1", "--format=%s")
	_ = os.RemoveAll(filepath.Join(dir, ".git"))
	return Commit{SHA: sha, Message: msg}, nil
}

func gitOut(ctx context.Context, dir string, args ...string) (string, error) {
	out, err := exec.CommandContext(ctx, "git", append([]string{"-C", dir}, args...)...).Output()
	return strings.TrimSpace(string(out)), err
}

const maxZipBytes = 512 << 20 // 512 MB uncompressed

// ExtractZip unpacks an uploaded archive into dir with zip-slip protection.
// If the archive contains a single top-level folder (as GitHub's
// "Download ZIP" does), that folder becomes the project root.
func ExtractZip(data []byte, dir string) (string, error) {
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return "", userErr("Uploaded file is not a valid zip archive")
	}
	var total int64
	tops := map[string]bool{}
	for _, f := range zr.File {
		// Normalise separators: archives made on Windows (e.g. PowerShell's
		// Compress-Archive) may use backslashes.
		name := strings.ReplaceAll(f.Name, "\\", "/")
		if strings.HasPrefix(name, "__MACOSX/") {
			continue
		}
		clean := filepath.Clean(filepath.FromSlash(name))
		if filepath.IsAbs(clean) || strings.HasPrefix(clean, ".."+string(filepath.Separator)) || clean == ".." {
			return "", userErr("Zip archive contains an unsafe path: %s", f.Name)
		}
		tops[strings.SplitN(name, "/", 2)[0]] = true
		target := filepath.Join(dir, clean)
		if f.FileInfo().IsDir() {
			if err := os.MkdirAll(target, 0o755); err != nil {
				return "", err
			}
			continue
		}
		if !f.Mode().IsRegular() {
			continue
		}
		total += int64(f.UncompressedSize64)
		if total > maxZipBytes {
			return "", userErr("Zip archive is larger than 512 MB when extracted")
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return "", err
		}
		rc, err := f.Open()
		if err != nil {
			return "", userErr("Corrupt zip entry %s", f.Name)
		}
		w, err := os.Create(target)
		if err != nil {
			rc.Close()
			return "", err
		}
		_, err = io.Copy(w, io.LimitReader(rc, maxZipBytes))
		rc.Close()
		w.Close()
		if err != nil {
			return "", err
		}
	}
	if len(tops) == 1 {
		for t := range tops {
			if st, err := os.Stat(filepath.Join(dir, t)); err == nil && st.IsDir() {
				return filepath.Join(dir, t), nil
			}
		}
	}
	return dir, nil
}
