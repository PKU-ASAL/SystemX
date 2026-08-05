package main

import (
	"fmt"
	"net/url"
	"strings"
)

func queryManagerArtifactsAPI(base string, args []string) ([]byte, error) {
	q := url.Values{}
	form := map[string]string{}
	var filePath, artifactID string
	upload, activate, revoke := false, false, false
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--upload":
			upload = true
		case "--activate":
			activate = true
		case "--revoke":
			revoke = true
		case "--artifact-id":
			i++
			if i < len(args) {
				artifactID = args[i]
			}
		case "--file":
			i++
			if i < len(args) {
				filePath = args[i]
			}
		case "--tenant-id":
			i++
			if i < len(args) {
				q.Set("tenant_id", args[i])
				form["tenant_id"] = args[i]
			}
		case "--name", "--kind", "--version", "--os", "--arch", "--status", "--actor":
			key := strings.TrimPrefix(args[i], "--")
			i++
			if i < len(args) {
				if key == "kind" || key == "status" {
					q.Set(key, args[i])
				}
				form[key] = args[i]
			}
		}
	}
	if upload {
		return httpPostMultipart(base+"/api/v1/artifacts", filePath, form)
	}
	if activate || revoke {
		if artifactID == "" {
			return nil, fmt.Errorf("--artifact-id is required")
		}
		action := "activate"
		if revoke {
			action = "revoke"
		}
		suffix := ""
		if tenantID := q.Get("tenant_id"); tenantID != "" {
			suffix = "?tenant_id=" + url.QueryEscape(tenantID)
		}
		return httpPostJSON(base+"/api/v1/artifacts/"+artifactID+"/"+action+suffix, map[string]any{})
	}
	return httpGet(base + "/api/v1/artifacts?" + q.Encode())
}

func queryManagerChannelsAPI(base string, args []string) ([]byte, error) {
	q := url.Values{}
	req := map[string]any{}
	upsert := false
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--upsert":
			upsert = true
		case "--tenant-id":
			i++
			if i < len(args) {
				q.Set("tenant_id", args[i])
				req["tenant_id"] = args[i]
			}
		case "--channel":
			i++
			if i < len(args) {
				req["channel"] = args[i]
			}
		case "--artifact-id":
			i++
			if i < len(args) {
				req["artifact_id"] = args[i]
			}
		case "--actor":
			i++
			if i < len(args) {
				req["actor"] = args[i]
			}
		}
	}
	if upsert {
		return httpPostJSON(base+"/api/v1/channels", req)
	}
	return httpGet(base + "/api/v1/channels?" + q.Encode())
}

func queryManagerArtifacts(base string, args []string) ([]byte, error) {
	if len(args) == 0 {
		args = []string{"list"}
	}
	switch args[0] {
	case "list":
		return queryManagerAPI(base, append([]string{"artifacts"}, managerArgsAfterAction(args, "list")...))
	case "upload":
		return queryManagerAPI(base, append([]string{"artifacts", "--upload"}, args[1:]...))
	case "activate":
		return queryManagerAPI(base, append([]string{"artifacts", "--activate"}, args[1:]...))
	case "revoke":
		return queryManagerAPI(base, append([]string{"artifacts", "--revoke"}, args[1:]...))
	default:
		return nil, fmt.Errorf("unknown manager artifacts command %q", args[0])
	}
}

func queryManagerChannels(base string, args []string) ([]byte, error) {
	if len(args) == 0 {
		args = []string{"list"}
	}
	switch args[0] {
	case "list":
		return queryManagerAPI(base, append([]string{"channels"}, managerArgsAfterAction(args, "list")...))
	case "upsert":
		return queryManagerAPI(base, append([]string{"channels", "--upsert"}, args[1:]...))
	default:
		return nil, fmt.Errorf("unknown manager channels command %q", args[0])
	}
}
