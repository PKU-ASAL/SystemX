package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	controlplanev1 "github.com/sysarmor/sysarmor-next-project/packages/contracts/proto/controlplane/v1"
)

const maxEnrollmentTokenBytes = 4096

type enrollmentCLIOptions struct {
	managerURL    string
	token         string
	uploadHistory bool
}

func parseEnrollmentArgs(defaultManagerURL string, args []string) (enrollmentCLIOptions, error) {
	if len(args) == 0 || args[0] != "enroll" {
		return enrollmentCLIOptions{}, fmt.Errorf("enroll command is required")
	}
	flags := flag.NewFlagSet("enroll", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	managerURL := flags.String("manager-url", defaultManagerURL, "Manager HTTP URL")
	token := flags.String("token", "", "one-time enrollment token")
	tokenFile := flags.String("token-file", "", "file containing one-time enrollment token")
	uploadHistory := flags.Bool("upload-history", false, "upload local history created before enrollment")
	_ = flags.Duration("timeout", 0, "maximum time to wait for manager policy activation")
	managerOwned := make(map[string]*string, 4)
	for _, name := range []string{"tenant", "agent-id", "gateway", "gateway-server-name"} {
		managerOwned[name] = flags.String(name, "", "configured by the Manager enrollment")
	}
	if err := flags.Parse(args[1:]); err != nil {
		return enrollmentCLIOptions{}, err
	}
	if flags.NArg() != 0 {
		return enrollmentCLIOptions{}, fmt.Errorf("unexpected enroll arguments: %s", strings.Join(flags.Args(), " "))
	}
	for name, value := range managerOwned {
		if strings.TrimSpace(*value) != "" {
			return enrollmentCLIOptions{}, fmt.Errorf("--%s is configured by the Manager enrollment and cannot be set on the Agent", name)
		}
	}
	if strings.TrimSpace(*token) != "" && strings.TrimSpace(*tokenFile) != "" {
		return enrollmentCLIOptions{}, fmt.Errorf("use only one of --token and --token-file")
	}
	resolvedToken := strings.TrimSpace(*token)
	if strings.TrimSpace(*tokenFile) != "" {
		data, err := readEnrollmentTokenFile(*tokenFile)
		if err != nil {
			return enrollmentCLIOptions{}, err
		}
		resolvedToken = data
	}
	if strings.TrimSpace(*managerURL) == "" || resolvedToken == "" {
		return enrollmentCLIOptions{}, fmt.Errorf("enroll requires --manager-url and either --token or --token-file")
	}
	return enrollmentCLIOptions{managerURL: strings.TrimSpace(*managerURL), token: resolvedToken, uploadHistory: *uploadHistory}, nil
}

func readEnrollmentTokenFile(path string) (string, error) {
	file, err := os.Open(strings.TrimSpace(path))
	if err != nil {
		return "", fmt.Errorf("open enrollment token file: %w", err)
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return "", fmt.Errorf("stat enrollment token file: %w", err)
	}
	if !info.Mode().IsRegular() || info.Size() > maxEnrollmentTokenBytes {
		return "", fmt.Errorf("enrollment token file must be a regular file no larger than %d bytes", maxEnrollmentTokenBytes)
	}
	data, err := io.ReadAll(io.LimitReader(file, maxEnrollmentTokenBytes+1))
	if err != nil {
		return "", fmt.Errorf("read enrollment token file: %w", err)
	}
	token := strings.TrimSpace(string(data))
	if token == "" {
		return "", fmt.Errorf("enrollment token file is empty")
	}
	return token, nil
}

func enrollLocalAgent(ctx context.Context, client controlplanev1.AgentControlPlaneServiceClient, reqCtx *controlplanev1.RequestContext, managerURL string, args []string) ([]byte, error) {
	opts, err := parseEnrollmentArgs(managerURL, args)
	if err != nil {
		return nil, err
	}
	resp, err := client.Enroll(ctx, &controlplanev1.EnrollRequest{
		Context: reqCtx, ManagerUrl: opts.managerURL, EnrollmentToken: opts.token, UploadHistory: opts.uploadHistory,
	})
	if err != nil {
		return nil, err
	}
	switch resp.GetStatus() {
	case "applied":
		return marshalProtoJSON(resp)
	case "pending":
		return waitForManagedEnrollment(ctx, client, reqCtx, resp)
	default:
		return nil, fmt.Errorf("enrollment rejected: %s", resp.GetMessage())
	}
}

func waitForManagedEnrollment(ctx context.Context, client controlplanev1.AgentControlPlaneServiceClient, reqCtx *controlplanev1.RequestContext, pending *controlplanev1.ControlAck) ([]byte, error) {
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	for {
		health, err := client.Health(ctx, &controlplanev1.HealthRequest{Context: reqCtx})
		if err != nil {
			return nil, fmt.Errorf("check enrollment activation: %w", err)
		}
		if health.GetLocalStore().GetMode() == "managed" {
			return marshalProtoJSON(&controlplanev1.ControlAck{
				RequestId: pending.GetRequestId(), TenantId: health.GetTenantId(), AgentId: health.GetAgentId(), Status: "applied",
				Message: "enrollment completed after manager endpoint policy activation", PolicyId: health.GetPolicyId(), PolicyVersion: health.GetPolicyVersion(),
			})
		}
		select {
		case <-ctx.Done():
			return nil, fmt.Errorf("wait for manager endpoint policy activation: %w", ctx.Err())
		case <-ticker.C:
		}
	}
}
