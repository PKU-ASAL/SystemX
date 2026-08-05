package store

import (
	"fmt"
	"sort"
	"strings"
	"time"
)

func (s *Store) UpsertArtifact(artifact Artifact) Artifact {
	artifact = normalizeArtifact(artifact)
	if artifact.ArtifactID == "" || artifact.Name == "" || artifact.Kind == "" || artifact.Version == "" || artifact.SHA256 == "" {
		return Artifact{}
	}
	now := time.Now().UTC()
	if artifact.UpdatedAt.IsZero() {
		artifact.UpdatedAt = now
	}
	if artifact.Status == "" {
		artifact.Status = "draft"
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for i, existing := range s.Artifacts {
		if existing.TenantID == artifact.TenantID && existing.ArtifactID == artifact.ArtifactID {
			if artifact.CreatedAt.IsZero() {
				artifact.CreatedAt = existing.CreatedAt
			}
			if artifact.CreatedAt.IsZero() {
				artifact.CreatedAt = now
			}
			s.Artifacts[i] = artifact
			return cloneArtifact(artifact)
		}
	}
	if artifact.CreatedAt.IsZero() {
		artifact.CreatedAt = now
	}
	s.Artifacts = append(s.Artifacts, artifact)
	return cloneArtifact(artifact)
}

func (s *Store) ListArtifacts(tenantID, kind, status string) []Artifact {
	if backend, ctx := s.backendCtx(); backend != nil {
		if artifacts, err := backend.ListArtifacts(ctx, tenantID, kind, status); err == nil {
			return artifacts
		}
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]Artifact, 0, len(s.Artifacts))
	for _, artifact := range s.Artifacts {
		if tenantID != "" && artifact.TenantID != tenantID {
			continue
		}
		if kind != "" && artifact.Kind != kind {
			continue
		}
		if status != "" && artifact.Status != status {
			continue
		}
		out = append(out, cloneArtifact(artifact))
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].TenantID == out[j].TenantID {
			return out[i].CreatedAt.Before(out[j].CreatedAt)
		}
		return out[i].TenantID < out[j].TenantID
	})
	return out
}

func (s *Store) ListArtifactsWithError(tenantID, kind, status string) ([]Artifact, error) {
	if backend, ctx := s.backendCtx(); backend != nil {
		artifacts, err := backend.ListArtifacts(ctx, tenantID, kind, status)
		if err != nil {
			return nil, fmt.Errorf("list artifacts: %w", err)
		}
		return artifacts, nil
	}
	return s.ListArtifacts(tenantID, kind, status), nil
}

func (s *Store) GetArtifact(tenantID, artifactID string) (Artifact, bool) {
	tenantID = strings.TrimSpace(tenantID)
	artifactID = strings.TrimSpace(artifactID)
	if artifactID == "" {
		return Artifact{}, false
	}
	if tenantID == "" {
		tenantID = "default"
	}
	if backend, ctx := s.backendCtx(); backend != nil {
		if artifact, ok, err := backend.GetArtifact(ctx, tenantID, artifactID); err == nil {
			return artifact, ok
		}
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, artifact := range s.Artifacts {
		if artifact.TenantID == tenantID && artifact.ArtifactID == artifactID {
			return cloneArtifact(artifact), true
		}
	}
	return Artifact{}, false
}

func (s *Store) GetArtifactWithError(tenantID, artifactID string) (Artifact, bool, error) {
	tenantID = strings.TrimSpace(tenantID)
	if tenantID == "" {
		tenantID = "default"
	}
	artifactID = strings.TrimSpace(artifactID)
	if artifactID == "" {
		return Artifact{}, false, nil
	}
	if backend, ctx := s.backendCtx(); backend != nil {
		artifact, ok, err := backend.GetArtifact(ctx, tenantID, artifactID)
		if err != nil {
			return Artifact{}, false, fmt.Errorf("get artifact: %w", err)
		}
		return artifact, ok, nil
	}
	artifact, ok := s.GetArtifact(tenantID, artifactID)
	return artifact, ok, nil
}

func normalizeArtifact(artifact Artifact) Artifact {
	artifact.ArtifactID = strings.TrimSpace(artifact.ArtifactID)
	artifact.TenantID = strings.TrimSpace(artifact.TenantID)
	if artifact.TenantID == "" {
		artifact.TenantID = "default"
	}
	artifact.Name = strings.TrimSpace(artifact.Name)
	artifact.Kind = strings.TrimSpace(artifact.Kind)
	artifact.Version = strings.TrimSpace(artifact.Version)
	artifact.OS = strings.TrimSpace(artifact.OS)
	artifact.Arch = strings.TrimSpace(artifact.Arch)
	artifact.SHA256 = strings.TrimSpace(artifact.SHA256)
	artifact.Status = strings.TrimSpace(artifact.Status)
	artifact.StoragePath = strings.TrimSpace(artifact.StoragePath)
	artifact.CreatedBy = strings.TrimSpace(artifact.CreatedBy)
	artifact.Metadata = cloneStringMap(artifact.Metadata)
	return artifact
}

func cloneArtifact(artifact Artifact) Artifact {
	artifact.Metadata = cloneStringMap(artifact.Metadata)
	return artifact
}

func normalizeChannel(channel ArtifactChannel) ArtifactChannel {
	channel.TenantID = strings.TrimSpace(channel.TenantID)
	if channel.TenantID == "" {
		channel.TenantID = "default"
	}
	channel.Channel = strings.TrimSpace(channel.Channel)
	channel.ArtifactID = strings.TrimSpace(channel.ArtifactID)
	channel.CreatedBy = strings.TrimSpace(channel.CreatedBy)
	return channel
}

func (s *Store) UpsertChannel(channel ArtifactChannel) ArtifactChannel {
	channel = normalizeChannel(channel)
	if channel.Channel == "" || channel.ArtifactID == "" {
		return ArtifactChannel{}
	}
	now := time.Now().UTC()
	if channel.UpdatedAt.IsZero() {
		channel.UpdatedAt = now
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for i, existing := range s.Channels {
		if existing.TenantID == channel.TenantID && existing.Channel == channel.Channel {
			if channel.CreatedAt.IsZero() {
				channel.CreatedAt = existing.CreatedAt
			}
			if channel.CreatedAt.IsZero() {
				channel.CreatedAt = now
			}
			s.Channels[i] = channel
			return channel
		}
	}
	if channel.CreatedAt.IsZero() {
		channel.CreatedAt = now
	}
	s.Channels = append(s.Channels, channel)
	return channel
}

func (s *Store) ListChannels(tenantID string) []ArtifactChannel {
	if backend, ctx := s.backendCtx(); backend != nil {
		if channels, err := backend.ListChannels(ctx, tenantID); err == nil {
			return channels
		}
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]ArtifactChannel, 0, len(s.Channels))
	for _, channel := range s.Channels {
		if tenantID != "" && channel.TenantID != tenantID {
			continue
		}
		out = append(out, channel)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].TenantID == out[j].TenantID {
			return out[i].Channel < out[j].Channel
		}
		return out[i].TenantID < out[j].TenantID
	})
	return out
}

func (s *Store) ListChannelsWithError(tenantID string) ([]ArtifactChannel, error) {
	if backend, ctx := s.backendCtx(); backend != nil {
		channels, err := backend.ListChannels(ctx, tenantID)
		if err != nil {
			return nil, fmt.Errorf("list channels: %w", err)
		}
		return channels, nil
	}
	return s.ListChannels(tenantID), nil
}

func (s *Store) GetChannel(tenantID, channelName string) (ArtifactChannel, bool) {
	tenantID = strings.TrimSpace(tenantID)
	channelName = strings.TrimSpace(channelName)
	if tenantID == "" {
		tenantID = "default"
	}
	if channelName == "" {
		return ArtifactChannel{}, false
	}
	if backend, ctx := s.backendCtx(); backend != nil {
		if channel, ok, err := backend.GetChannel(ctx, tenantID, channelName); err == nil {
			return channel, ok
		}
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, channel := range s.Channels {
		if channel.TenantID == tenantID && channel.Channel == channelName {
			return channel, true
		}
	}
	return ArtifactChannel{}, false
}

func (s *Store) GetChannelWithError(tenantID, channelName string) (ArtifactChannel, bool, error) {
	tenantID = strings.TrimSpace(tenantID)
	if tenantID == "" {
		tenantID = "default"
	}
	channelName = strings.TrimSpace(channelName)
	if channelName == "" {
		return ArtifactChannel{}, false, nil
	}
	if backend, ctx := s.backendCtx(); backend != nil {
		channel, ok, err := backend.GetChannel(ctx, tenantID, channelName)
		if err != nil {
			return ArtifactChannel{}, false, fmt.Errorf("get channel: %w", err)
		}
		return channel, ok, nil
	}
	channel, ok := s.GetChannel(tenantID, channelName)
	return channel, ok, nil
}
