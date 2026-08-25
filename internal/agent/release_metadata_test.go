package agent

import "testing"

func TestReleaseDeploymentMetadataRoundTrip(t *testing.T) {
	installDir := t.TempDir()
	want := ReleaseDeploymentMetadata{
		Version:     "0.3.6",
		Owner:       "example",
		Repository:  "simple-go-server",
		Ref:         "v0.3.6",
		ArtifactURL: "https://github.com/example/simple-go-server/releases/download/v0.3.6/app.zip",
	}
	if err := WriteReleaseDeploymentMetadata(installDir, want); err != nil {
		t.Fatalf("write metadata: %v", err)
	}
	got, err := ReadReleaseDeploymentMetadata(installDir, want.Version)
	if err != nil {
		t.Fatalf("read metadata: %v", err)
	}
	if got.Owner != want.Owner || got.Repository != want.Repository || got.Ref != want.Ref || got.ArtifactURL != want.ArtifactURL {
		t.Fatalf("metadata = %+v, want %+v", got, want)
	}
}
