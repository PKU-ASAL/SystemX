package entity

import (
	"strings"

	signalv1 "github.com/sysarmor/sysarmor-next-project/api/proto/signal/v1"
)

func Normalize(ref *signalv1.EntityRef) *signalv1.EntityRef {
	if ref == nil {
		return nil
	}
	kind := strings.TrimSpace(strings.ToLower(ref.GetKind()))
	key := strings.TrimSpace(ref.GetKey())
	role := strings.TrimSpace(strings.ToLower(ref.GetRole()))
	switch kind {
	case "file":
		key = ensurePrefix(key, "file:")
	case "socket":
		key = ensurePrefix(key, "socket:")
	case "process":
		key = ensurePrefix(key, "process:")
	case "container":
		key = ensurePrefix(key, "container:")
	case "user":
		key = ensurePrefix(key, "user:")
	case "token":
		key = ensurePrefix(key, "token:")
	}
	return &signalv1.EntityRef{Kind: kind, Key: key, Role: role}
}

func Unique(refs []*signalv1.EntityRef) []*signalv1.EntityRef {
	seen := map[string]bool{}
	out := make([]*signalv1.EntityRef, 0, len(refs))
	for _, ref := range refs {
		normalized := Normalize(ref)
		if normalized == nil || normalized.GetKind() == "" || normalized.GetKey() == "" {
			continue
		}
		key := normalized.GetKind() + "\x00" + normalized.GetKey() + "\x00" + normalized.GetRole()
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, normalized)
	}
	return out
}

func ensurePrefix(key, prefix string) string {
	if key == "" || strings.HasPrefix(key, prefix) {
		return key
	}
	return prefix + key
}
