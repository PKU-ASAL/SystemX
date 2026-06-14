package tetragon

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

type BundleConfig struct {
	BundleDir    string
	TetraPath    string
	TetragonPath string
}

type BundleManifest struct {
	Version string                  `json:"version"`
	Files   map[string]ManifestFile `json:"files"`
}

type ManifestFile struct {
	SHA256 string `json:"sha256"`
}

type BundleVerification struct {
	Version      string
	TetraPath    string
	TetragonPath string
}

func VerifyBundle(cfg BundleConfig) (BundleVerification, error) {
	if strings.TrimSpace(cfg.BundleDir) == "" {
		return BundleVerification{}, fmt.Errorf("tetragon bundle_dir is required")
	}
	manifestPath := filepath.Join(cfg.BundleDir, "manifest.json")
	data, err := os.ReadFile(manifestPath)
	if err != nil {
		return BundleVerification{}, fmt.Errorf("read tetragon bundle manifest: %w", err)
	}
	var manifest BundleManifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		return BundleVerification{}, fmt.Errorf("decode tetragon bundle manifest: %w", err)
	}
	if manifest.Version == "" {
		return BundleVerification{}, fmt.Errorf("tetragon bundle manifest version is required")
	}
	tetragonPath := firstNonEmpty(cfg.TetragonPath, filepath.Join(cfg.BundleDir, "bin", "tetragon"))
	tetraPath := firstNonEmpty(cfg.TetraPath, filepath.Join(cfg.BundleDir, "bin", "tetra"))
	if err := verifyManifestFile(cfg.BundleDir, manifest, "bin/tetragon", tetragonPath); err != nil {
		return BundleVerification{}, err
	}
	if err := verifyManifestFile(cfg.BundleDir, manifest, "bin/tetra", tetraPath); err != nil {
		return BundleVerification{}, err
	}
	return BundleVerification{
		Version:      manifest.Version,
		TetraPath:    tetraPath,
		TetragonPath: tetragonPath,
	}, nil
}

func verifyManifestFile(bundleDir string, manifest BundleManifest, relPath, actualPath string) error {
	file, ok := manifest.Files[relPath]
	if !ok {
		return fmt.Errorf("tetragon bundle manifest missing %s", relPath)
	}
	want := strings.ToLower(strings.TrimSpace(file.SHA256))
	if want == "" {
		return fmt.Errorf("tetragon bundle manifest missing sha256 for %s", relPath)
	}
	if !filepath.IsAbs(actualPath) {
		actualPath = filepath.Join(bundleDir, actualPath)
	}
	got, err := sha256File(actualPath)
	if err != nil {
		return fmt.Errorf("verify tetragon bundle file %s: %w", relPath, err)
	}
	if got != want {
		return fmt.Errorf("verify tetragon bundle file %s: checksum mismatch got=%s want=%s", relPath, got, want)
	}
	return nil
}

func sha256File(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}
