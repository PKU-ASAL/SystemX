package managerapi

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/sysarmor/sysarmor-next-project/internal/distribution"
	"github.com/sysarmor/sysarmor-next-project/internal/store"
)

func (s *Server) artifacts(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		q := r.URL.Query()
		writeJSON(w, map[string]any{"artifacts": s.store.ListArtifacts(q.Get("tenant_id"), q.Get("kind"), q.Get("status"))})
	case http.MethodPost:
		if !s.requireOperator(w, r, "admin") {
			return
		}
		artifact, err := s.receiveArtifact(r)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		artifact.CreatedBy = s.actorFromRequest(r, r.FormValue("actor"))
		artifact = s.store.UpsertArtifact(artifact)
		if artifact.ArtifactID == "" {
			http.Error(w, "create artifact failed", http.StatusBadRequest)
			return
		}
		if err := s.store.Save(); err != nil {
			http.Error(w, fmt.Sprintf("save store: %v", err), http.StatusInternalServerError)
			return
		}
		writeJSON(w, map[string]any{"artifact": artifact, "download_url": artifactDownloadURL(r, artifact.ArtifactID)})
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) artifactByID(w http.ResponseWriter, r *http.Request) {
	rest := strings.TrimPrefix(r.URL.Path, "/api/v1/artifacts/")
	parts := strings.Split(strings.Trim(rest, "/"), "/")
	if len(parts) == 0 || parts[0] == "" {
		http.NotFound(w, r)
		return
	}
	artifactID := parts[0]
	tenantID := r.URL.Query().Get("tenant_id")
	if len(parts) == 1 {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		artifact, ok := s.store.GetArtifact(tenantID, artifactID)
		if !ok {
			http.NotFound(w, r)
			return
		}
		writeJSON(w, artifact)
		return
	}
	if len(parts) != 2 {
		http.NotFound(w, r)
		return
	}
	switch parts[1] {
	case "download":
		s.downloadArtifact(w, r, tenantID, artifactID)
	case "activate", "revoke":
		s.updateArtifactStatus(w, r, tenantID, artifactID, parts[1])
	default:
		http.NotFound(w, r)
	}
}

func (s *Server) receiveArtifact(r *http.Request) (store.Artifact, error) {
	if err := r.ParseMultipartForm(64 << 20); err != nil {
		return store.Artifact{}, fmt.Errorf("parse artifact upload: %w", err)
	}
	file, header, err := r.FormFile("file")
	if err != nil {
		return store.Artifact{}, fmt.Errorf("file is required: %w", err)
	}
	defer file.Close()
	name := strings.TrimSpace(r.FormValue("name"))
	kind := strings.TrimSpace(r.FormValue("kind"))
	version := strings.TrimSpace(r.FormValue("version"))
	if name == "" || kind == "" || version == "" {
		return store.Artifact{}, fmt.Errorf("name, kind, and version are required")
	}
	tenantID := strings.TrimSpace(r.FormValue("tenant_id"))
	if tenantID == "" {
		tenantID = "default"
	}
	now := time.Now().UTC()
	artifactID := "art-" + now.Format("20060102T150405Z") + "-" + randomSuffix()
	dir := filepath.Join(s.artifactDir, safePathSegment(tenantID), safePathSegment(kind), safePathSegment(name), safePathSegment(version), safePathSegment(artifactID))
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return store.Artifact{}, fmt.Errorf("create artifact dir: %w", err)
	}
	filename := safeFilename(header.Filename)
	if filename == "" {
		filename = "artifact.tar.gz"
	}
	tmp, err := os.CreateTemp(dir, ".upload-*.tmp")
	if err != nil {
		return store.Artifact{}, fmt.Errorf("create artifact temp file: %w", err)
	}
	tmpName := tmp.Name()
	hasher := sha256.New()
	size, copyErr := io.Copy(io.MultiWriter(tmp, hasher), file)
	closeErr := tmp.Close()
	if copyErr != nil {
		_ = os.Remove(tmpName)
		return store.Artifact{}, fmt.Errorf("write artifact: %w", copyErr)
	}
	if closeErr != nil {
		_ = os.Remove(tmpName)
		return store.Artifact{}, fmt.Errorf("close artifact: %w", closeErr)
	}
	finalPath := filepath.Join(dir, filename)
	if err := os.Rename(tmpName, finalPath); err != nil {
		_ = os.Remove(tmpName)
		return store.Artifact{}, fmt.Errorf("store artifact: %w", err)
	}
	status := strings.TrimSpace(r.FormValue("status"))
	if status == "" {
		status = "draft"
	}
	metadata := map[string]string{}
	if kind == "agent" {
		inspected, err := distribution.InspectTarGz(finalPath, s.artifactPub)
		if err != nil {
			_ = os.Remove(finalPath)
			return store.Artifact{}, fmt.Errorf("inspect agent distribution: %w", err)
		}
		if inspected.Manifest.Name != name || inspected.Manifest.Version != version {
			_ = os.Remove(finalPath)
			return store.Artifact{}, fmt.Errorf("artifact form metadata does not match distribution manifest")
		}
		if osName := strings.TrimSpace(r.FormValue("os")); osName != "" && inspected.Manifest.OS != osName {
			_ = os.Remove(finalPath)
			return store.Artifact{}, fmt.Errorf("artifact os does not match distribution manifest")
		}
		if arch := strings.TrimSpace(r.FormValue("arch")); arch != "" && inspected.Manifest.Arch != arch {
			_ = os.Remove(finalPath)
			return store.Artifact{}, fmt.Errorf("artifact arch does not match distribution manifest")
		}
		metadata["manifest_schema"] = inspected.Manifest.SchemaVersion
		metadata["manifest_entrypoint"] = inspected.Manifest.Entrypoint
		metadata["manifest_systemd_unit"] = inspected.Manifest.SystemdUnit
		metadata["manifest_signed"] = strconv.FormatBool(inspected.Signed)
	}
	return store.Artifact{
		ArtifactID:  artifactID,
		TenantID:    tenantID,
		Name:        name,
		Kind:        kind,
		Version:     version,
		OS:          r.FormValue("os"),
		Arch:        r.FormValue("arch"),
		SHA256:      hex.EncodeToString(hasher.Sum(nil)),
		SizeBytes:   size,
		Status:      status,
		StoragePath: finalPath,
		CreatedAt:   now,
		UpdatedAt:   now,
		Metadata:    metadata,
	}, nil
}

func (s *Server) downloadArtifact(w http.ResponseWriter, r *http.Request, tenantID, artifactID string) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	artifact, ok := s.store.GetArtifact(tenantID, artifactID)
	if !ok || artifact.Status == "revoked" {
		http.NotFound(w, r)
		return
	}
	if artifact.StoragePath == "" {
		if url := artifactInstallURL(r, artifact); url != artifactDownloadURL(r, artifact.ArtifactID) {
			http.Redirect(w, r, url, http.StatusFound)
			return
		}
		http.Error(w, "artifact has no storage path", http.StatusNotFound)
		return
	}
	if !pathWithinDir(s.artifactDir, artifact.StoragePath) {
		http.Error(w, "artifact storage path escapes artifact dir", http.StatusInternalServerError)
		return
	}
	w.Header().Set("X-SysArmor-Artifact-ID", artifact.ArtifactID)
	w.Header().Set("X-SysArmor-Artifact-SHA256", artifact.SHA256)
	http.ServeFile(w, r, artifact.StoragePath)
}

func (s *Server) updateArtifactStatus(w http.ResponseWriter, r *http.Request, tenantID, artifactID, action string) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !s.requireOperator(w, r, "admin") {
		return
	}
	artifact, ok := s.store.GetArtifact(tenantID, artifactID)
	if !ok {
		http.NotFound(w, r)
		return
	}
	if action == "activate" {
		artifact.Status = "active"
	} else {
		artifact.Status = "revoked"
	}
	artifact.UpdatedAt = time.Now().UTC()
	artifact = s.store.UpsertArtifact(artifact)
	if err := s.store.Save(); err != nil {
		http.Error(w, fmt.Sprintf("save store: %v", err), http.StatusInternalServerError)
		return
	}
	writeJSON(w, artifact)
}

func randomSuffix() string {
	var b [6]byte
	if _, err := rand.Read(b[:]); err != nil {
		return strconv.FormatInt(time.Now().UnixNano(), 36)
	}
	return hex.EncodeToString(b[:])
}

func safePathSegment(v string) string {
	v = strings.TrimSpace(v)
	if v == "" {
		return "_"
	}
	var b strings.Builder
	for _, r := range v {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '.' || r == '-' || r == '_' {
			b.WriteRune(r)
		} else {
			b.WriteByte('_')
		}
	}
	return b.String()
}

func safeFilename(name string) string {
	name = filepath.Base(strings.TrimSpace(name))
	if name == "." || name == "/" {
		return ""
	}
	return safePathSegment(name)
}

func pathWithinDir(dir, path string) bool {
	root, err := filepath.Abs(dir)
	if err != nil {
		return false
	}
	target, err := filepath.Abs(path)
	if err != nil {
		return false
	}
	rel, err := filepath.Rel(root, target)
	if err != nil {
		return false
	}
	return rel == "." || (!strings.HasPrefix(rel, ".."+string(filepath.Separator)) && rel != "..")
}
