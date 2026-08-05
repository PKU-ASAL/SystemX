package tetragon

import (
	"bytes"
	"fmt"
	"strconv"
	"strings"

	"github.com/sysarmor/sysarmor-next-project/packages/eventmodel"
	"github.com/sysarmor/sysarmor-next-project/packages/sensor-sdk/contract"
)

func buildTracingPolicy(intent contract.CollectionIntent) []byte {
	var out bytes.Buffer
	out.WriteString("apiVersion: cilium.io/v1alpha1\n")
	out.WriteString("kind: TracingPolicy\n")
	out.WriteString("metadata:\n")
	out.WriteString("  name: ")
	out.WriteString(fmt.Sprintf("%q", runtimeTracingPolicyName))
	out.WriteString("\n")
	out.WriteString("spec:\n")
	out.WriteString("  kprobes:\n")
	if intentHasAnyBehavior(intent, eventmodel.BehaviorProcessExec.String(), eventmodel.BehaviorProcessFork.String()) {
		prefixes := mergeFilterStrings(
			behaviorFilter(intent, eventmodel.BehaviorProcessExec.String()).BinaryPrefixes,
			behaviorFilter(intent, eventmodel.BehaviorProcessFork.String()).BinaryPrefixes,
		)
		out.WriteString(`  - call: "security_bprm_creds_from_file"
    syscall: false
    args:
    - index: 0
      type: "nop"
    - index: 1
      type: "file"
`)
		if len(prefixes) > 0 || len(intent.NamespaceSelectors) > 0 {
			out.WriteString("    selectors:\n")
			out.WriteString("    -\n")
			writeNamespaceSelectors(&out, intent.NamespaceSelectors, "      ")
		}
		if len(prefixes) > 0 {
			out.WriteString(`      matchArgs:
      - index: 1
        operator: "Prefix"
        values:
`)
			for _, prefix := range prefixes {
				out.WriteString("        - ")
				out.WriteString(fmt.Sprintf("%q", prefix))
				out.WriteString("\n")
			}
		}
	}
	if intentHasBehavior(intent, eventmodel.BehaviorProcessExit.String()) {
		out.WriteString(`  - call: "do_exit"
    syscall: false
    args:
    - index: 0
      type: "int"
`)
	}
	if intentHasBehavior(intent, eventmodel.BehaviorNetworkConnect.String()) {
		filter := behaviorFilter(intent, eventmodel.BehaviorNetworkConnect.String())
		families := filter.SocketFamilies
		if len(families) == 0 {
			families = []string{"AF_INET", "AF_INET6"}
		}
		out.WriteString(`  - call: "security_socket_connect"
    syscall: false
    args:
    - index: 1
      type: "sockaddr"
    - index: 2
      type: "int"
    selectors:
    -
`)
		writeMatchBinaries(&out, filter.BinaryPrefixes, "      ")
		writeNamespaceSelectors(&out, intent.NamespaceSelectors, "      ")
		out.WriteString(`      matchArgs:
      - index: 1
        operator: "Family"
        values:
`)
		for _, family := range families {
			out.WriteString("        - ")
			out.WriteString(fmt.Sprintf("%q", family))
			out.WriteString("\n")
		}
	}
	fileSelectors := filePermissionSelectors(intent)
	if len(fileSelectors) > 0 {
		out.WriteString(`  - call: "security_file_permission"
    syscall: false
    return: true
    args:
    - index: 0
      type: "file"
    - index: 1
      type: "int"
    returnArg:
      index: 0
      type: "int"
    selectors:
`)
		for _, selector := range fileSelectors {
			writeFilePermissionSelector(&out, selector)
		}
	}
	return out.Bytes()
}

type filePermissionSelector struct {
	Behavior           string
	Access             int32
	BinaryPrefixes     []string
	FilePrefixes       []string
	NamespaceSelectors []contract.NamespaceSelector
}

func filePermissionSelectors(intent contract.CollectionIntent) []filePermissionSelector {
	var selectors []filePermissionSelector
	add := func(behavior string, access int32) {
		if !intentHasBehavior(intent, behavior) {
			return
		}
		filter := behaviorFilter(intent, behavior)
		prefixes := filter.FilePrefixes
		if len(prefixes) == 0 {
			prefixes = defaultFilePrefixesForBehavior(behavior)
		}
		selectors = append(selectors, filePermissionSelector{
			Behavior:           behavior,
			Access:             access,
			BinaryPrefixes:     filter.BinaryPrefixes,
			FilePrefixes:       prefixes,
			NamespaceSelectors: append([]contract.NamespaceSelector(nil), intent.NamespaceSelectors...),
		})
	}
	add(eventmodel.BehaviorFileOpen.String(), 0)
	add(eventmodel.BehaviorFileRead.String(), 4)
	add(eventmodel.BehaviorFileWrite.String(), 2)
	add(eventmodel.BehaviorFileChmod.String(), 2)
	return selectors
}

func defaultFilePrefixesForBehavior(behavior string) []string {
	switch eventmodel.NormalizeBehavior(behavior) {
	case eventmodel.BehaviorFileRead:
		return []string{"/root/.ssh", "/var/run/secrets", "/etc/passwd", "/etc/shadow", "/etc/sudoers"}
	case eventmodel.BehaviorFileWrite, eventmodel.BehaviorFileChmod:
		return []string{"/dev/shm", "/tmp", "/var/tmp"}
	default:
		return []string{"/root/.ssh", "/var/run/secrets", "/etc/passwd"}
	}
}

func writeFilePermissionSelector(out *bytes.Buffer, selector filePermissionSelector) {
	out.WriteString("    -\n")
	writeMatchBinaries(out, selector.BinaryPrefixes, "      ")
	writeNamespaceSelectors(out, selector.NamespaceSelectors, "      ")
	out.WriteString(`      matchArgs:
      - index: 0
        operator: "Prefix"
        values:
`)
	for _, prefix := range mergeFilterStrings(selector.FilePrefixes) {
		out.WriteString("        - ")
		out.WriteString(fmt.Sprintf("%q", prefix))
		out.WriteString("\n")
	}
	if selector.Access != 0 {
		out.WriteString(`      - index: 1
        operator: "Equal"
        values:
`)
		out.WriteString("        - ")
		out.WriteString(fmt.Sprintf("%q", strconv.FormatInt(int64(selector.Access), 10)))
		out.WriteString("\n")
	}
}

func writeMatchBinaries(out *bytes.Buffer, prefixes []string, indent string) {
	prefixes = mergeFilterStrings(prefixes)
	if len(prefixes) == 0 {
		return
	}
	out.WriteString(indent)
	out.WriteString("matchBinaries:\n")
	out.WriteString(indent)
	out.WriteString("- operator: \"Prefix\"\n")
	out.WriteString(indent)
	out.WriteString("  values:\n")
	for _, prefix := range prefixes {
		out.WriteString(indent)
		out.WriteString("  - ")
		out.WriteString(fmt.Sprintf("%q", prefix))
		out.WriteString("\n")
	}
}

func writeNamespaceSelectors(out *bytes.Buffer, selectors []contract.NamespaceSelector, indent string) {
	if len(selectors) == 0 {
		return
	}
	out.WriteString(indent)
	out.WriteString("matchNamespaces:\n")
	for _, selector := range selectors {
		values := mergeFilterStrings(selector.Values)
		if selector.Namespace == "" || len(values) == 0 {
			continue
		}
		out.WriteString(indent)
		out.WriteString("- namespace: ")
		out.WriteString(selector.Namespace)
		out.WriteString("\n")
		out.WriteString(indent)
		out.WriteString("  operator: \"In\"\n")
		out.WriteString(indent)
		out.WriteString("  values:\n")
		for _, value := range values {
			out.WriteString(indent)
			out.WriteString("  - ")
			out.WriteString(fmt.Sprintf("%q", value))
			out.WriteString("\n")
		}
	}
}

func intentHasAnyBehavior(intent contract.CollectionIntent, behaviors ...string) bool {
	for _, behavior := range behaviors {
		if intentHasBehavior(intent, behavior) {
			return true
		}
	}
	return false
}

func intentHasBehavior(intent contract.CollectionIntent, behavior string) bool {
	behavior = eventmodel.NormalizeBehavior(behavior).String()
	for _, got := range intent.Behaviors {
		if eventmodel.NormalizeBehavior(got).String() == behavior {
			return true
		}
	}
	return false
}

func behaviorFilter(intent contract.CollectionIntent, behavior string) contract.CollectionBehaviorFilter {
	behavior = eventmodel.NormalizeBehavior(behavior).String()
	for _, filter := range intent.BehaviorFilters {
		if eventmodel.NormalizeBehavior(filter.Behavior).String() == behavior {
			return filter
		}
	}
	filter := contract.CollectionBehaviorFilter{Behavior: behavior}
	switch behavior {
	case eventmodel.BehaviorProcessExec.String(), eventmodel.BehaviorProcessFork.String():
		filter.BinaryPrefixes = intent.BinaryPrefixes
	case eventmodel.BehaviorNetworkConnect.String():
		filter.BinaryPrefixes = intent.BinaryPrefixes
		filter.SocketFamilies = intent.SocketFamilies
		filter.SocketAddrs = intent.SocketAddrs
		filter.SocketPorts = intent.SocketPorts
	case eventmodel.BehaviorFileOpen.String(), eventmodel.BehaviorFileRead.String(), eventmodel.BehaviorFileWrite.String(), eventmodel.BehaviorFileChmod.String():
		filter.BinaryPrefixes = intent.BinaryPrefixes
		filter.FilePrefixes = intent.FilePrefixes
	}
	return filter
}

func mergeFilterStrings(lists ...[]string) []string {
	seen := map[string]bool{}
	var out []string
	for _, list := range lists {
		for _, value := range list {
			value = strings.TrimSpace(value)
			if value == "" || seen[value] {
				continue
			}
			seen[value] = true
			out = append(out, value)
		}
	}
	return out
}
