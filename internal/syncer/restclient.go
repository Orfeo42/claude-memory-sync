package syncer

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"claude-memory-sync/internal/manifest"
)

const httpTimeout = 30 * time.Second

type restClient struct {
	baseURL    string
	token      string
	clientID   string
	httpClient *http.Client
}

func NewHTTPClient(serverURL, token, clientID string) HTTPClient {
	return &restClient{
		baseURL:    strings.TrimSuffix(serverURL, "/"),
		token:      token,
		clientID:   clientID,
		httpClient: &http.Client{Timeout: httpTimeout},
	}
}

func escapePath(path string) string {
	segments := strings.Split(path, "/")
	for i, s := range segments {
		segments[i] = url.PathEscape(s)
	}
	return strings.Join(segments, "/")
}

func (c *restClient) do(ctx context.Context, method, path string, body []byte) (*http.Response, error) {
	var reader io.Reader
	if body != nil {
		reader = bytes.NewReader(body)
	}

	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, reader)
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.token)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("do request: %w", err)
	}
	return resp, nil
}

func checkStatus(resp *http.Response, want int) error {
	if resp.StatusCode == want {
		return nil
	}
	body, _ := io.ReadAll(resp.Body)
	return fmt.Errorf("%w: status %d: %s", errRequestFailed, resp.StatusCode, string(body))
}

func decodeManifest(resp *http.Response) (manifest.Manifest, error) {
	var m manifest.Manifest
	if err := json.NewDecoder(resp.Body).Decode(&m); err != nil {
		return nil, fmt.Errorf("decode tree response: %w", err)
	}
	return m, nil
}

func (c *restClient) ClientTree(ctx context.Context) (manifest.Manifest, error) {
	resp, err := c.do(ctx, http.MethodGet, "/v1/clients/"+escapePath(c.clientID)+"/tree", nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if err := checkStatus(resp, http.StatusOK); err != nil {
		return nil, err
	}
	return decodeManifest(resp)
}

func (c *restClient) PutClientFile(ctx context.Context, path string, content []byte) error {
	resp, err := c.do(ctx, http.MethodPut, "/v1/clients/"+escapePath(c.clientID)+"/file/"+escapePath(path), content)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	return checkStatus(resp, http.StatusNoContent)
}

func (c *restClient) DeleteClientFile(ctx context.Context, path string) error {
	resp, err := c.do(ctx, http.MethodDelete, "/v1/clients/"+escapePath(c.clientID)+"/file/"+escapePath(path), nil)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	return checkStatus(resp, http.StatusNoContent)
}

func (c *restClient) CanonicalTree(ctx context.Context) (manifest.Manifest, error) {
	resp, err := c.do(ctx, http.MethodGet, "/v1/canonical/tree", nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if err := checkStatus(resp, http.StatusOK); err != nil {
		return nil, err
	}
	return decodeManifest(resp)
}

func (c *restClient) GetCanonicalFile(ctx context.Context, path string) ([]byte, error) {
	resp, err := c.do(ctx, http.MethodGet, "/v1/canonical/file/"+escapePath(path), nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if err := checkStatus(resp, http.StatusOK); err != nil {
		return nil, err
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read file response: %w", err)
	}
	return data, nil
}
