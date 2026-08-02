package distribution

import (
	"archive/tar"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestInspectTarGzRejectsUnsignedExtraEntry(t *testing.T) {
	manifest, file := testManifestAndFile(t)
	path := writeTestArchive(t, []testArchiveEntry{
		regularEntry("manifest.json", mustJSON(t, manifest)),
		regularEntry("manifest.sig", []byte("placeholder")),
		regularEntry(file.Path, []byte("hello")),
		regularEntry("extra.sh", []byte("#!/bin/sh\nexit 0\n")),
	})

	_, err := InspectTarGz(path, nil)
	if err == nil || !strings.Contains(err.Error(), "unexpected archive entry") {
		t.Fatalf("InspectTarGz extra entry error = %v, want unexpected archive entry", err)
	}
}

func TestInspectTarGzRejectsSymlinkEntry(t *testing.T) {
	manifest, file := testManifestAndFile(t)
	path := writeTestArchive(t, []testArchiveEntry{
		regularEntry("manifest.json", mustJSON(t, manifest)),
		regularEntry("manifest.sig", []byte("placeholder")),
		regularEntry(file.Path, []byte("hello")),
		{
			header: &tar.Header{Name: "artifact-public.pem", Typeflag: tar.TypeSymlink, Linkname: "/etc/sysarmor-target"},
		},
	})

	_, err := InspectTarGz(path, nil)
	if err == nil || !strings.Contains(err.Error(), "unsupported archive entry") {
		t.Fatalf("InspectTarGz symlink error = %v, want unsupported archive entry", err)
	}
}

func TestInspectTarGzRejectsFileLargerThanLimit(t *testing.T) {
	manifest, file := testManifestAndFile(t)
	path := writeTestArchive(t, []testArchiveEntry{
		regularEntry("manifest.json", mustJSON(t, manifest)),
		regularEntry("manifest.sig", []byte("placeholder")),
		regularEntry(file.Path, []byte("hello")),
	})

	_, err := inspectTarGzWithLimits(path, nil, inspectLimits{MaxFileBytes: 4})
	if err == nil || !strings.Contains(err.Error(), "exceeds archive file size limit") {
		t.Fatalf("InspectTarGz oversized file error = %v, want archive file size limit", err)
	}
}

func testManifestAndFile(t *testing.T) (Manifest, FileSpec) {
	t.Helper()
	sum := sha256.Sum256([]byte("hello"))
	file := FileSpec{Path: "install.sh", SHA256: hex.EncodeToString(sum[:])}
	return Manifest{
		SchemaVersion: SchemaVersion,
		Name:          "sysarmor-agent",
		Version:       "v1",
		OS:            "linux",
		Arch:          "amd64",
		Entrypoint:    "install.sh",
		SystemdUnit:   "sysarmor-agent.service",
		Install:       InstallSpec{AgentHome: "/opt/sysarmor/agent", ConfigPath: "/etc/sysarmor/agent.yaml", PolicyDir: "/etc/sysarmor/policy", RuntimeSocket: "/run/sysarmor.sock"},
		Files:         []FileSpec{file},
	}, file
}

type testArchiveEntry struct {
	header *tar.Header
	body   []byte
}

func regularEntry(name string, body []byte) testArchiveEntry {
	return testArchiveEntry{header: &tar.Header{Name: name, Typeflag: tar.TypeReg, Mode: 0o644, Size: int64(len(body))}, body: body}
}

func writeTestArchive(t *testing.T, entries []testArchiveEntry) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "artifact.tar.gz")
	file, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	gz := gzip.NewWriter(file)
	tw := tar.NewWriter(gz)
	for _, entry := range entries {
		if err := tw.WriteHeader(entry.header); err != nil {
			t.Fatal(err)
		}
		if len(entry.body) > 0 {
			if _, err := tw.Write(entry.body); err != nil {
				t.Fatal(err)
			}
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	return path
}

func mustJSON(t *testing.T, v any) []byte {
	t.Helper()
	raw, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}
