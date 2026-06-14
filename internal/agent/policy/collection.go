package policy

import (
	"fmt"
	"os"
	"regexp"
	"strings"

	eventv1 "github.com/sysarmor/sysarmor-next-project/api/proto/event/v1"
	"github.com/sysarmor/sysarmor-next-project/internal/sensor/contract"
)

var eventKinds = map[string]eventv1.EventKind{
	"EXEC":    eventv1.EventKind_EVENT_KIND_EXEC,
	"EXIT":    eventv1.EventKind_EVENT_KIND_EXIT,
	"FORK":    eventv1.EventKind_EVENT_KIND_FORK,
	"OPEN":    eventv1.EventKind_EVENT_KIND_OPEN,
	"WRITE":   eventv1.EventKind_EVENT_KIND_WRITE,
	"CHMOD":   eventv1.EventKind_EVENT_KIND_CHMOD,
	"CONNECT": eventv1.EventKind_EVENT_KIND_CONNECT,
}

var kindsPattern = regexp.MustCompile(`(?m)^\s*kinds:\s*\[([^\]]*)\]`)

func LoadCollectionIntent(path string, observeOnly bool) (contract.CollectionIntent, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return contract.CollectionIntent{}, err
	}
	intent, err := ParseCollectionIntent(string(data), observeOnly)
	if err != nil {
		return contract.CollectionIntent{}, fmt.Errorf("%s: %w", path, err)
	}
	return intent, nil
}

func ParseCollectionIntent(data string, observeOnly bool) (contract.CollectionIntent, error) {
	match := kindsPattern.FindStringSubmatch(data)
	if len(match) != 2 {
		return contract.CollectionIntent{}, fmt.Errorf("collection kinds not found")
	}
	seen := map[eventv1.EventKind]bool{}
	var kinds []eventv1.EventKind
	for _, raw := range strings.Split(match[1], ",") {
		name := strings.ToUpper(strings.Trim(strings.TrimSpace(raw), `"'`))
		if name == "" {
			continue
		}
		kind, ok := eventKinds[name]
		if !ok {
			continue
		}
		if seen[kind] {
			continue
		}
		seen[kind] = true
		kinds = append(kinds, kind)
	}
	if len(kinds) == 0 {
		return contract.CollectionIntent{}, fmt.Errorf("collection policy has no supported event kinds")
	}
	return contract.CollectionIntent{EventKinds: kinds, ObserveOnly: observeOnly}, nil
}
