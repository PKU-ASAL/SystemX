package main

import (
	"fmt"
	"net/url"
)

func queryManagerIncidentAPI(base string, args []string) ([]byte, error) {
	switch args[0] {
	case "incidents":
		q := url.Values{}
		for i := 1; i < len(args); i++ {
			switch args[i] {
			case "--label":
				i++
				if i < len(args) {
					addLabelQuery(q, args[i])
				}
			case "--limit":
				i++
				if i < len(args) {
					q.Set("limit", args[i])
				}
			case "--offset":
				i++
				if i < len(args) {
					q.Set("offset", args[i])
				}
			}
		}
		return httpGet(base + "/api/v1/incidents?" + q.Encode())
	case "incident-evidence":
		q := url.Values{}
		for i := 1; i < len(args); i++ {
			switch args[i] {
			case "--label":
				i++
				if i < len(args) {
					addLabelQuery(q, args[i])
				}
			case "--incident-id":
				i++
				if i < len(args) {
					q.Set("incident_id", args[i])
				}
			case "--path-from":
				i++
				if i < len(args) {
					q.Set("path_from", args[i])
				}
			case "--path-to":
				i++
				if i < len(args) {
					q.Set("path_to", args[i])
				}
			case "--seed":
				i++
				if i < len(args) {
					q.Set("seed", args[i])
				}
			case "--hops":
				i++
				if i < len(args) {
					q.Set("hops", args[i])
				}
			}
		}
		return httpGet(base + "/api/v1/incident-evidence?" + q.Encode())
	case "incident-evidence-attach":
		req := map[string]any{}
		labels := map[string]string{}
		node := map[string]string{}
		edge := map[string]string{}
		for i := 1; i < len(args); i++ {
			switch args[i] {
			case "--label":
				i++
				if i < len(args) {
					addLabelMap(labels, args[i])
				}
			case "--incident-id":
				i++
				if i < len(args) {
					req["incident_id"] = args[i]
				}
			case "--node-id":
				i++
				if i < len(args) {
					node["id"] = args[i]
				}
			case "--node-kind":
				i++
				if i < len(args) {
					node["kind"] = args[i]
				}
			case "--node-label":
				i++
				if i < len(args) {
					node["label"] = args[i]
				}
			case "--edge-id":
				i++
				if i < len(args) {
					edge["id"] = args[i]
				}
			case "--edge-from":
				i++
				if i < len(args) {
					edge["from"] = args[i]
				}
			case "--edge-to":
				i++
				if i < len(args) {
					edge["to"] = args[i]
				}
			case "--edge-kind":
				i++
				if i < len(args) {
					edge["kind"] = args[i]
				}
			}
		}
		evidence := map[string]any{}
		if len(node) > 0 {
			evidence["nodes"] = []map[string]string{node}
		}
		if len(edge) > 0 {
			evidence["edges"] = []map[string]string{edge}
		}
		if len(labels) > 0 {
			req["labels"] = labels
		}
		req["evidence"] = evidence
		return httpPostJSON(base+"/api/v1/incident-evidence", req)
	case "incident-lifecycle":
		req := map[string]any{}
		labels := map[string]string{}
		for i := 1; i < len(args); i++ {
			switch args[i] {
			case "--label":
				i++
				if i < len(args) {
					addLabelMap(labels, args[i])
				}
			case "--incident-id":
				i++
				if i < len(args) {
					req["incident_id"] = args[i]
				}
			case "--status":
				i++
				if i < len(args) {
					req["status"] = args[i]
				}
			case "--reason":
				i++
				if i < len(args) {
					req["reason"] = args[i]
				}
			case "--actor":
				i++
				if i < len(args) {
					req["actor"] = args[i]
				}
			}
		}
		if len(labels) > 0 {
			req["labels"] = labels
		}
		return httpPostJSON(base+"/api/v1/incident-lifecycle", req)
	case "incident-merge":
		req := map[string]string{}
		for i := 1; i < len(args); i++ {
			switch args[i] {
			case "--target-incident-id":
				i++
				if i < len(args) {
					req["target_incident_id"] = args[i]
				}
			case "--source-incident-id":
				i++
				if i < len(args) {
					req["source_incident_id"] = args[i]
				}
			}
		}
		return httpPostJSON(base+"/api/v1/incident-merge", req)
	default:
		return nil, fmt.Errorf("unknown command %q", args[0])
	}
}
