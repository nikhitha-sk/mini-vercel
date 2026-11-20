package ids

import "testing"

func TestDeploymentLabelRoundTrip(t *testing.T) {
	id := Deployment()
	label := DeploymentLabel(id)
	back, ok := DeploymentFromLabel(label)
	if !ok || back != id {
		t.Fatalf("round trip failed: %s -> %s -> %s", id, label, back)
	}
}

func TestDeploymentFromLabelRejectsProjects(t *testing.T) {
	for _, l := range []string{"portfolio", "dpl-short", "dpl-", "xyz-abcdefghij"} {
		if _, ok := DeploymentFromLabel(l); ok {
			t.Errorf("%q should not parse as a deployment", l)
		}
	}
}
