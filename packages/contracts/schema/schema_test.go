package schema

import (
	"errors"
	"testing"
)

func TestValidateDataPlaneCurrentSchema(t *testing.T) {
	legacy, err := ValidateDataPlane(DataPlaneCurrent)
	if err != nil || legacy {
		t.Fatalf("legacy=%t err=%v", legacy, err)
	}
}

func TestValidateDataPlaneRejectsEmptySchema(t *testing.T) {
	legacy, err := ValidateDataPlane("")
	var unsupported *UnsupportedVersionError
	if legacy || !errors.As(err, &unsupported) {
		t.Fatalf("legacy=%t err=%v", legacy, err)
	}
}

func TestValidateDataPlaneRejectsUnsupportedSchema(t *testing.T) {
	legacy, err := ValidateDataPlane("sysarmor.dataplane/v9")
	var unsupported *UnsupportedVersionError
	if legacy || !errors.As(err, &unsupported) {
		t.Fatalf("legacy=%t err=%v", legacy, err)
	}
	if unsupported.Version != "sysarmor.dataplane/v9" {
		t.Fatalf("version=%q", unsupported.Version)
	}
}

func TestDataPlaneCompatibilityWindow(t *testing.T) {
	for _, tc := range []struct {
		name, version string
		legacy, fails bool
	}{
		{"current", DataPlaneCurrent, false, false},
		{"empty", "", false, true},
		{"unknown", "sysarmor.dataplane/v2", false, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			legacy, err := ValidateDataPlane(tc.version)
			if legacy != tc.legacy || (err != nil) != tc.fails {
				t.Fatalf("legacy=%t err=%v", legacy, err)
			}
		})
	}
}
