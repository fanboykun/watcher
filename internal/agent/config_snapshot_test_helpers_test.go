package agent

import "testing"

func captureConfigSnapshotsForTest(t *testing.T, wcfg *WatcherConfig, versions ...string) {
	t.Helper()
	for _, version := range versions {
		if err := CaptureConfigSnapshot(wcfg, version, SnapshotSourceDeployment); err != nil {
			t.Fatalf("capture config snapshot for %s: %v", version, err)
		}
	}
}
