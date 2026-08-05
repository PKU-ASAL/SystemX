package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestMergeReleaseConfigCommandWritesMergedConfig(t *testing.T) {
	dir := t.TempDir()
	existingPath := filepath.Join(dir, "existing.yaml")
	releasePath := filepath.Join(dir, "release.yaml")
	outputPath := filepath.Join(dir, "merged.yaml")
	existing := `local:
  state_path: /custom/state
sensor:
  backend: tetragon
  mode: managed
policy:
  path: /custom/policy.json
content:
  path: /custom/content
  trust_keys: "old=key"
`
	release := `local:
  state_path: /var/lib/sysarmor/agent
sensor:
  backend: tetragon
  mode: managed
policy:
  path: /etc/sysarmor/agent/policy.json
content:
  default_path: /opt/sysarmor/agent/content/default
  trust_keys: "release=new-key"
`
	if err := os.WriteFile(existingPath, []byte(existing), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(releasePath, []byte(release), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := mergeReleaseConfigCommand([]string{"--existing", existingPath, "--release", releasePath, "--output", outputPath}); err != nil {
		t.Fatal(err)
	}
	merged, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(merged), `path: "/custom/content"`) || !strings.Contains(string(merged), `trust_keys: "release=new-key"`) {
		t.Fatalf("merged config =\n%s", merged)
	}
	info, err := os.Stat(outputPath)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("output mode = %o, want 600", info.Mode().Perm())
	}
}
