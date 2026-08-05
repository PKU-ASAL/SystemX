package schema

import "fmt"

const DataPlaneCurrent = "sysarmor.dataplane/v1"

type UnsupportedVersionError struct {
	Version string
}

func (e *UnsupportedVersionError) Error() string {
	return fmt.Sprintf("unsupported data plane schema version %q", e.Version)
}

func ValidateDataPlane(version string) (bool, error) {
	switch version {
	case DataPlaneCurrent:
		return false, nil
	default:
		return false, &UnsupportedVersionError{Version: version}
	}
}
