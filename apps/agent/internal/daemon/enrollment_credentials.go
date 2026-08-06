package daemon

import (
	"fmt"
	"os"
)

func rollbackEnrollmentFailure(paths credentialPaths, created bool, cause error) error {
	if err := rollbackEnrollmentCredentials(paths, created); err != nil {
		return fmt.Errorf("%w; rollback credentials: %v", cause, err)
	}
	return cause
}

func removeCredentials(paths credentialPaths) []string {
	var failures []string
	for _, path := range []string{paths.CA, paths.Certificate, paths.Key} {
		if path == "" {
			continue
		}
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			failures = append(failures, fmt.Sprintf("%s: %v", path, err))
		}
	}
	return failures
}
