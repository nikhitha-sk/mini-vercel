// Package ids generates short, URL- and DNS-safe identifiers.
package ids

import (
	"crypto/rand"
	"strings"
)

const alphabet = "abcdefghijklmnopqrstuvwxyz0123456789"

func random(n int) string {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	for i := range b {
		b[i] = alphabet[int(b[i])%len(alphabet)]
	}
	return string(b)
}

func Deployment() string { return "dpl_" + random(10) }
func Project() string    { return "prj_" + random(10) }
func User() string       { return "usr_" + random(10) }
func EnvVar() string     { return "env_" + random(10) }
func Secret() string     { return random(32) }

// DeploymentLabel converts a deployment id into its DNS label (dpl_x -> dpl-x).
func DeploymentLabel(id string) string { return strings.Replace(id, "_", "-", 1) }

// DeploymentFromLabel is the inverse of DeploymentLabel. ok is false when
// the label does not look like a deployment.
func DeploymentFromLabel(label string) (string, bool) {
	if !strings.HasPrefix(label, "dpl-") || len(label) != len("dpl-")+10 {
		return "", false
	}
	return "dpl_" + label[4:], true
}
