package tetragon

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestVerifyBundleChecksManifestAndBinaries(t *testing.T) {
	dir := writeBundle(t, "v1.2.3", "tetragon-bin", "tetra-bin")
	got, err := VerifyBundle(BundleConfig{BundleDir: dir})
	if err != nil {
		t.Fatalf("VerifyBundle() error = %v", err)
	}
	if got.Version != "v1.2.3" {
		t.Fatalf("version = %q", got.Version)
	}
	if got.TetragonPath != filepath.Join(dir, "bin", "tetragon") || got.TetraPath != filepath.Join(dir, "bin", "tetra") {
		t.Fatalf("paths = %+v", got)
	}
}

func TestVerifyBundleRejectsChecksumMismatch(t *testing.T) {
	dir := writeBundle(t, "v1.2.3", "tetragon-bin", "tetra-bin")
	if err := os.WriteFile(filepath.Join(dir, "bin", "tetra"), []byte("changed"), 0o755); err != nil {
		t.Fatal(err)
	}
	_, err := VerifyBundle(BundleConfig{BundleDir: dir})
	if err == nil || !strings.Contains(err.Error(), "checksum mismatch") {
		t.Fatalf("VerifyBundle() error = %v, want checksum mismatch", err)
	}
}

func TestCapabilityVerifiesBundleAndUpdatesHealth(t *testing.T) {
	dir := writeBundle(t, "v1.2.3", "tetragon-bin", "tetra-bin")
	backend := NewBackendWithBundle("policy.yaml", "", "", BundleConfig{BundleDir: dir})
	capability, err := backend.Capability(nil)
	if err != nil {
		t.Fatalf("Capability() error = %v", err)
	}
	if capability.Version != "v1.2.3" {
		t.Fatalf("capability version = %q", capability.Version)
	}
	health, err := backend.Health(nil)
	if err != nil {
		t.Fatalf("Health() error = %v", err)
	}
	if !health.Installed || health.Version != "v1.2.3" {
		t.Fatalf("health = %+v", health)
	}
}

func writeBundle(t *testing.T, version, tetragonContent, tetraContent string) string {
	t.Helper()
	dir := t.TempDir()
	binDir := filepath.Join(dir, "bin")
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(binDir, "tetragon"), []byte(tetragonContent), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(binDir, "tetra"), []byte(tetraContent), 0o755); err != nil {
		t.Fatal(err)
	}
	manifest := fmt.Sprintf(`{
  "version": %q,
  "files": {
    "bin/tetragon": {"sha256": %q},
    "bin/tetra": {"sha256": %q}
  }
}`, version, sha256Hex(tetragonContent), sha256Hex(tetraContent))
	if err := os.WriteFile(filepath.Join(dir, "manifest.json"), []byte(manifest), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}

func sha256Hex(data string) string {
	sum := sha256.Sum256([]byte(data))
	return hex.EncodeToString(sum[:])
}
