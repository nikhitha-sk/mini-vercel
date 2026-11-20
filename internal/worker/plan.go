package worker

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/nikhitha-sk/mini-vercel/internal/builder"
	"github.com/nikhitha-sk/mini-vercel/internal/logstream"
	"github.com/nikhitha-sk/mini-vercel/internal/queue"
)

// buildPlan is the effective build configuration: user overrides from the
// project settings win, everything else comes from auto-detection.
type buildPlan struct {
	framework string
	node      string
	image     string
	rootDir   string
	install   string
	build     string
	override  string   // user-configured output directory ("" = auto)
	outputs   []string // candidate output directories, in order
}

func (b buildPlan) outputLabel() string {
	if b.override != "" {
		return b.override
	}
	if len(b.outputs) > 0 {
		return b.outputs[0]
	}
	return "."
}

var nodeImageRe = regexp.MustCompile(`^node:(\d+)`)

func (w *Worker) plan(p queue.BuildPayload, srcDir string, logger *logstream.Logger) (buildPlan, error) {
	bp := buildPlan{rootDir: strings.Trim(p.Build.RootDir, "/"), image: p.Build.Image}
	if bp.image == "" {
		bp.image = w.Cfg.BuildImage
	}

	projectDir := filepath.Join(srcDir, filepath.FromSlash(bp.rootDir))
	if bp.rootDir != "" {
		if st, err := os.Stat(projectDir); err != nil || !st.IsDir() {
			if p.Source.Type == "zip" {
				logger.Line("stderr", fmt.Sprintf("Warning: root directory %q is not in the uploaded archive, using the archive root", bp.rootDir))
				bp.rootDir, projectDir = "", srcDir
			} else {
				return bp, &builder.UserError{Msg: fmt.Sprintf("Root directory %q was not found in the repository", bp.rootDir)}
			}
		}
	}

	det := builder.Detect(projectDir)
	bp.framework = det.Framework

	// Node version: the repo's .nvmrc / engines.node, unless an image was pinned.
	if det.NodeMajor > 0 && bp.image == w.Cfg.BuildImage && strings.HasPrefix(bp.image, "node:") {
		bp.image = fmt.Sprintf("node:%d-alpine", det.NodeMajor)
	}
	if m := nodeImageRe.FindStringSubmatch(bp.image); m != nil {
		bp.node = m[1]
	}

	// Install / build commands.
	bp.build = det.Build
	if o := strings.TrimSpace(p.Build.Command); o != "" {
		switch {
		case !det.HasPackageJSON && isPackageScript(o):
			logger.Line("stderr", fmt.Sprintf("Warning: no package.json found, skipping build command %q (deploying as a static site)", o))
			bp.build = ""
		case det.HasPackageJSON && !det.HasBuildScript && isRunBuild(o):
			logger.Line("stderr", "Warning: package.json has no \"build\" script, skipping the build step")
		default:
			bp.build = o
		}
	}
	switch {
	case strings.TrimSpace(p.Build.Install) != "":
		bp.install = strings.TrimSpace(p.Build.Install)
	case det.HasPackageJSON && bp.build != "":
		bp.install = det.Install
	}

	// Output folder candidates.
	bp.override = strings.Trim(strings.TrimSpace(p.Build.OutputDir), "/")
	if bp.override != "" {
		bp.outputs = append(bp.outputs, bp.override)
	}
	bp.outputs = append(bp.outputs, det.OutputDirs...)

	desc := det.FrameworkName
	if det.HasPackageJSON {
		desc += " · " + det.PackageManager
	}
	if bp.node != "" {
		desc += " · Node " + bp.node
	}
	logger.System("Detected " + desc)
	for _, warn := range det.Warnings {
		logger.Line("stderr", "Warning: "+warn)
	}
	return bp, nil
}

func isPackageScript(cmd string) bool {
	for _, p := range []string{"npm ", "npx ", "yarn", "pnpm", "bun ", "node "} {
		if strings.HasPrefix(cmd, p) {
			return true
		}
	}
	return false
}

var runBuildRe = regexp.MustCompile(`^(npm|yarn|pnpm|bun)( run)? build$`)

func isRunBuild(cmd string) bool { return runBuildRe.MatchString(strings.TrimSpace(cmd)) }
