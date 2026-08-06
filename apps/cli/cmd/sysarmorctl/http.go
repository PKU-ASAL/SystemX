package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"os"
	"strings"
)

func httpGet(url string) ([]byte, error) {
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	addAuthHeaders(req)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode >= 300 {
		return nil, fmt.Errorf("GET %s: %s: %s", url, resp.Status, string(body))
	}
	return body, nil
}

func addLabelQuery(q url.Values, raw string) {
	key, value, ok := strings.Cut(raw, "=")
	key = strings.TrimSpace(key)
	value = strings.TrimSpace(value)
	if !ok || key == "" {
		return
	}
	q.Add("label", key+"="+value)
}

func addLabelMap(labels map[string]string, raw string) {
	key, value, ok := strings.Cut(raw, "=")
	key = strings.TrimSpace(key)
	value = strings.TrimSpace(value)
	if !ok || key == "" {
		return
	}
	labels[key] = value
}

func httpPostJSON(url string, body any) ([]byte, error) {
	data, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	return httpPostRaw(url, data)
}

func httpPostMultipart(url, filePath string, fields map[string]string) ([]byte, error) {
	if strings.TrimSpace(filePath) == "" {
		return nil, fmt.Errorf("--file is required")
	}
	file, err := os.Open(filePath)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	for k, v := range fields {
		if err := writer.WriteField(k, v); err != nil {
			return nil, err
		}
	}
	part, err := writer.CreateFormFile("file", filePath)
	if err != nil {
		return nil, err
	}
	if _, err := io.Copy(part, file); err != nil {
		return nil, err
	}
	if err := writer.Close(); err != nil {
		return nil, err
	}
	req, err := http.NewRequest(http.MethodPost, url, &body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())
	addAuthHeaders(req)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	out, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 300 {
		return nil, fmt.Errorf("POST %s: %s: %s", url, resp.Status, string(out))
	}
	return out, nil
}

func httpPostRaw(url string, data []byte) ([]byte, error) {
	req, err := http.NewRequest(http.MethodPost, url, strings.NewReader(string(data)))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	addAuthHeaders(req)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	out, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 300 {
		return nil, fmt.Errorf("POST %s: %s: %s", url, resp.Status, string(out))
	}
	return out, nil
}

func addAuthHeaders(req *http.Request) {
	if token := os.Getenv("SYSARMOR_DEV_TOKEN"); token != "" {
		req.Header.Set("X-SysArmor-Agent-Token", token)
	}
	if token := strings.TrimSpace(os.Getenv("SYSARMOR_MANAGER_JWT")); token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
}
