package localapi

import (
	"encoding/json"
	"testing"

	controlplanev1 "github.com/sysarmor/sysarmor-next-project/packages/contracts/proto/controlplane/v1"
)

func TestHandlerDebugProfileOwnsRuntimeProfile(t *testing.T) {
	handler := NewHandler(Dependencies{})
	response, err := handler.DebugProfile(t.Context(), &controlplanev1.DebugProfileRequest{
		ProfileType: "runtime", Label: "test-profile",
	})
	if err != nil {
		t.Fatal(err)
	}
	var payload map[string]any
	if err := json.Unmarshal(response.GetProfile(), &payload); err != nil {
		t.Fatal(err)
	}
	if response.GetProfileType() != "runtime" || response.GetLabel() != "test-profile" || payload["label"] != "test-profile" {
		t.Fatalf("response=%+v payload=%+v", response, payload)
	}
}
