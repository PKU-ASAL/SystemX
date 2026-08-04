package managerapi

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/sysarmor/sysarmor-next-project/internal/store"
)

const maxUnenrollmentCompletionBody = 16 << 10

var errUnenrollmentCompletionTooLarge = errors.New("unenrollment completion body is too large")

type unenrollmentCompletionRequest struct {
	SchemaVersion     string `json:"schema_version"`
	TenantID          string `json:"tenant_id"`
	AgentID           string `json:"agent_id"`
	EnrollmentID      string `json:"enrollment_id"`
	CertificateSerial string `json:"certificate_serial"`
	RevocationReceipt string `json:"revocation_receipt"`
	CompletionToken   string `json:"completion_token"`
}

func (s *Server) unenrollmentCompletion(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	request, err := decodeUnenrollmentCompletion(r)
	if err != nil {
		if errors.Is(err, errUnenrollmentCompletionTooLarge) {
			writeAPIError(w, http.StatusRequestEntityTooLarge, "request_too_large", "Request body too large")
			return
		}
		writeAPIError(w, http.StatusBadRequest, "invalid_request", "Invalid request")
		return
	}
	tokenHash := sha256.Sum256([]byte(request.CompletionToken))
	record, found, err := s.store.CompleteAgentUnenrollment(request.TenantID, request.AgentID, request.EnrollmentID,
		request.CertificateSerial, request.RevocationReceipt, hex.EncodeToString(tokenHash[:]), time.Now().UTC())
	if errors.Is(err, store.ErrConflict) || !found {
		writeAPIError(w, http.StatusUnauthorized, "unauthorized", "Unauthorized")
		return
	}
	if err != nil {
		writeAPIError(w, http.StatusInternalServerError, "store_error", "Store operation failed")
		return
	}
	writeJSON(w, map[string]any{"status": record.Status, "endpoint_completed_at": record.EndpointCompletedAt})
}

func decodeUnenrollmentCompletion(r *http.Request) (unenrollmentCompletionRequest, error) {
	var request unenrollmentCompletionRequest
	raw, err := io.ReadAll(io.LimitReader(r.Body, maxUnenrollmentCompletionBody+1))
	if err != nil {
		return unenrollmentCompletionRequest{}, err
	}
	if len(raw) > maxUnenrollmentCompletionBody {
		return unenrollmentCompletionRequest{}, errUnenrollmentCompletionTooLarge
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&request); err != nil {
		return unenrollmentCompletionRequest{}, err
	}
	if decoder.Decode(&struct{}{}) != io.EOF || request.SchemaVersion != "sysarmor.unenrollment-completion/v1" {
		return unenrollmentCompletionRequest{}, errors.New("invalid completion document")
	}
	values := []string{request.TenantID, request.AgentID, request.EnrollmentID, request.CertificateSerial, request.RevocationReceipt, request.CompletionToken}
	for _, value := range values {
		if strings.TrimSpace(value) == "" {
			return unenrollmentCompletionRequest{}, errors.New("incomplete completion document")
		}
	}
	return request, nil
}
