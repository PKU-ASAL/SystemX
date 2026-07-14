package schema

import (
	"bufio"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

var legacyAgentTestPatterns = []string{
	"/etc/sysarmor/agent.yaml",
	"/etc/sysarmor/policies",
	"/run/sysarmor/agent.sock",
	"\ndata_plane:",
	"\nmanager:",
	"\n  batch_size:",
	"\n  policy_path:",
}

func TestAgentTestAssetsUseCurrentSchema(t *testing.T) {
	root := repositoryRoot(t)
	testRoot := filepath.Join(root, "test")
	err := filepath.WalkDir(testRoot, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			if entry.Name() == ".results" {
				return filepath.SkipDir
			}
			if path == filepath.Join(testRoot, "environments", "vm-topology", "deploy", "platform") {
				return filepath.SkipDir
			}
			return nil
		}
		if filepath.Ext(path) != ".sh" && filepath.Ext(path) != ".md" && filepath.Base(path) != "Makefile" {
			return nil
		}
		raw, readErr := os.ReadFile(path)
		if readErr != nil {
			return readErr
		}
		for _, pattern := range legacyAgentTestPatterns {
			if strings.Contains(string(raw), pattern) {
				t.Errorf("legacy Agent test pattern %q in %s", pattern, filepath.ToSlash(path[len(root)+1:]))
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	validateCoverageInventory(t, filepath.Join(testRoot, "contracts", "agent-test-coverage.tsv"))
}

func validateCoverageInventory(t *testing.T, path string) {
	t.Helper()
	file, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()

	seen := map[string]bool{}
	scanner := bufio.NewScanner(file)
	for line := 1; scanner.Scan(); line++ {
		if line == 1 {
			continue
		}
		fields := strings.Split(scanner.Text(), "\t")
		if len(fields) != 5 || slicesContainEmpty(fields) {
			t.Errorf("%s:%d: expected five non-empty tab-separated fields", path, line)
			continue
		}
		key := fields[0] + "\t" + fields[2]
		if seen[key] {
			t.Errorf("%s:%d: duplicate source/assertion %q", path, line, key)
		}
		seen[key] = true
		if fields[4] != "legacy" && fields[4] != "covered" {
			t.Errorf("%s:%d: invalid status %q", path, line, fields[4])
		}
	}
	if err := scanner.Err(); err != nil {
		t.Fatal(err)
	}
}

func slicesContainEmpty(values []string) bool {
	for _, value := range values {
		if strings.TrimSpace(value) == "" {
			return true
		}
	}
	return false
}

func repositoryRoot(t *testing.T) string {
	t.Helper()
	root, err := filepath.Abs(filepath.Join("..", "..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	return root
}
