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

func TestValidateDataPlaneEmptySchemaAsLegacy(t *testing.T) {
	legacy, err := ValidateDataPlane("")
	if err != nil || !legacy {
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
