package client

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"claude-memory-sync/internal/domain"
	"claude-memory-sync/internal/pipeline"
)

const apiName = "pipeline"

type Client struct {
	baseURL    string
	token      string
	httpClient *http.Client
}

type CreateRequest struct {
	SourceFlow string          `json:"source_flow"`
	SourceRun  string          `json:"source_run"`
	Files      []pipeline.File `json:"files"`
}

type ListParams struct {
	Status *pipeline.Status
	Kind   *pipeline.Kind
	Before *time.Time
	Limit  int
}

type DecideRequest struct {
	Decision string `json:"decision"`
	Actor    string `json:"actor"`
	Reason   string `json:"reason,omitempty"`
}

type ReviseRequest struct {
	Actor string          `json:"actor"`
	Files []pipeline.File `json:"files"`
}

type actorRequest struct {
	Actor string `json:"actor"`
}

type claimRequest struct {
	Worker       string `json:"worker"`
	LeaseSeconds int    `json:"lease_seconds,omitempty"`
}

type appliedRequest struct {
	Actor string `json:"actor"`
	SHA   string `json:"sha"`
}

type failedRequest struct {
	Actor string `json:"actor"`
	Error string `json:"error"`
}

type expireRequest struct {
	Actor          string `json:"actor"`
	OlderThanHours int    `json:"older_than_hours"`
}

type expireResponse struct {
	Expired int `json:"expired"`
}

type listResponse struct {
	Proposals []pipeline.Proposal `json:"proposals"`
}

type rejectionsResponse struct {
	Rejections []pipeline.Rejection `json:"rejections"`
}

type AddRejectionRequest struct {
	Kind   pipeline.Kind `json:"kind"`
	Name   string        `json:"name"`
	Actor  string        `json:"actor"`
	Reason *string       `json:"reason,omitempty"`
}

type problemResponse struct {
	Detail string `json:"detail"`
}

func New(cfg Config, httpClient *http.Client) *Client {
	return &Client{
		baseURL:    strings.TrimRight(cfg.URL, "/"),
		token:      cfg.Token,
		httpClient: httpClient,
	}
}

func (c *Client) Create(ctx context.Context, req CreateRequest) (pipeline.Proposal, error) {
	body, err := json.Marshal(req)
	if err != nil {
		return pipeline.Proposal{}, domain.Error(err, "encode create request failed")
	}
	return c.proposalCall(ctx, "create", http.MethodPost, "/v1/proposals", body)
}

func (c *Client) List(ctx context.Context, params ListParams) ([]pipeline.Proposal, error) {
	query := url.Values{}
	if params.Status != nil {
		query.Set("status", string(*params.Status))
	}
	if params.Kind != nil {
		query.Set("kind", string(*params.Kind))
	}
	if params.Before != nil {
		query.Set("before", params.Before.Format(time.RFC3339))
	}
	if params.Limit > 0 {
		query.Set("limit", strconv.Itoa(params.Limit))
	}

	data, _, err := c.do(ctx, "list", http.MethodGet, "/v1/proposals?"+query.Encode(), nil)
	if err != nil {
		return nil, err
	}

	var resp listResponse
	if err := json.Unmarshal(data, &resp); err != nil {
		return nil, domain.Error(err, "decode list response failed", slog.String("operation", "list"))
	}
	if resp.Proposals == nil {
		return []pipeline.Proposal{}, nil
	}
	return resp.Proposals, nil
}

func (c *Client) Get(ctx context.Context, id string) (pipeline.ProposalDetail, error) {
	data, _, err := c.do(ctx, "get", http.MethodGet, proposalPath(id, ""), nil)
	if err != nil {
		return pipeline.ProposalDetail{}, err
	}

	var detail pipeline.ProposalDetail
	if err := json.Unmarshal(data, &detail); err != nil {
		return pipeline.ProposalDetail{}, domain.Error(err, "decode get response failed", slog.String("operation", "get"))
	}
	return detail, nil
}

func (c *Client) Decide(ctx context.Context, id string, req DecideRequest) (pipeline.Proposal, error) {
	body, err := json.Marshal(req)
	if err != nil {
		return pipeline.Proposal{}, domain.Error(err, "encode decide request failed")
	}
	return c.proposalCall(ctx, "decide", http.MethodPost, proposalPath(id, "/decision"), body)
}

func (c *Client) Revise(ctx context.Context, id string, req ReviseRequest) (pipeline.Proposal, error) {
	body, err := json.Marshal(req)
	if err != nil {
		return pipeline.Proposal{}, domain.Error(err, "encode revise request failed")
	}
	return c.proposalCall(ctx, "revise", http.MethodPut, proposalPath(id, "/files"), body)
}

func (c *Client) Retry(ctx context.Context, id, actor string) (pipeline.Proposal, error) {
	body, err := json.Marshal(actorRequest{Actor: actor})
	if err != nil {
		return pipeline.Proposal{}, domain.Error(err, "encode retry request failed")
	}
	return c.proposalCall(ctx, "retry", http.MethodPost, proposalPath(id, "/retry"), body)
}

func (c *Client) Claim(ctx context.Context, worker string, leaseSeconds int) (pipeline.Proposal, error) {
	body, err := json.Marshal(claimRequest{Worker: worker, LeaseSeconds: leaseSeconds})
	if err != nil {
		return pipeline.Proposal{}, domain.Error(err, "encode claim request failed")
	}

	data, status, err := c.do(ctx, "claim", http.MethodPost, "/v1/proposals/claim", body)
	if err != nil {
		return pipeline.Proposal{}, err
	}
	if status == http.StatusNoContent {
		return pipeline.Proposal{}, pipeline.ErrNothingToClaim
	}

	var proposal pipeline.Proposal
	if err := json.Unmarshal(data, &proposal); err != nil {
		return pipeline.Proposal{}, domain.Error(err, "decode claim response failed", slog.String("operation", "claim"))
	}
	return proposal, nil
}

func (c *Client) MarkApplied(ctx context.Context, id, actor, sha string) (pipeline.Proposal, error) {
	body, err := json.Marshal(appliedRequest{Actor: actor, SHA: sha})
	if err != nil {
		return pipeline.Proposal{}, domain.Error(err, "encode applied request failed")
	}
	return c.proposalCall(ctx, "mark_applied", http.MethodPost, proposalPath(id, "/applied"), body)
}

func (c *Client) MarkFailed(ctx context.Context, id, actor, errText string) (pipeline.Proposal, error) {
	body, err := json.Marshal(failedRequest{Actor: actor, Error: errText})
	if err != nil {
		return pipeline.Proposal{}, domain.Error(err, "encode failed request failed")
	}
	return c.proposalCall(ctx, "mark_failed", http.MethodPost, proposalPath(id, "/failed"), body)
}

func (c *Client) Expire(ctx context.Context, actor string, olderThanHours int) (int, error) {
	body, err := json.Marshal(expireRequest{Actor: actor, OlderThanHours: olderThanHours})
	if err != nil {
		return 0, domain.Error(err, "encode expire request failed")
	}

	data, _, err := c.do(ctx, "expire", http.MethodPost, "/v1/proposals/expire", body)
	if err != nil {
		return 0, err
	}

	var resp expireResponse
	if err := json.Unmarshal(data, &resp); err != nil {
		return 0, domain.Error(err, "decode expire response failed", slog.String("operation", "expire"))
	}
	return resp.Expired, nil
}

func (c *Client) Rejections(ctx context.Context, kind *pipeline.Kind) ([]pipeline.Rejection, error) {
	path := "/v1/rejections"
	if kind != nil {
		path += "?" + url.Values{"kind": {string(*kind)}}.Encode()
	}

	data, _, err := c.do(ctx, "rejections", http.MethodGet, path, nil)
	if err != nil {
		return nil, err
	}

	var resp rejectionsResponse
	if err := json.Unmarshal(data, &resp); err != nil {
		return nil, domain.Error(err, "decode rejections response failed", slog.String("operation", "rejections"))
	}
	if resp.Rejections == nil {
		return []pipeline.Rejection{}, nil
	}
	return resp.Rejections, nil
}

func (c *Client) AddRejection(ctx context.Context, req AddRejectionRequest) error {
	body, err := json.Marshal(req)
	if err != nil {
		return domain.Error(err, "encode add rejection request failed")
	}

	_, _, err = c.do(ctx, "add_rejection", http.MethodPost, "/v1/rejections", body)
	return err
}

func proposalPath(id, suffix string) string {
	return "/v1/proposals/" + url.PathEscape(id) + suffix
}

func (c *Client) proposalCall(ctx context.Context, operation, method, path string, body []byte) (pipeline.Proposal, error) {
	data, _, err := c.do(ctx, operation, method, path, body)
	if err != nil {
		return pipeline.Proposal{}, err
	}

	var proposal pipeline.Proposal
	if err := json.Unmarshal(data, &proposal); err != nil {
		return pipeline.Proposal{}, domain.Error(err, "decode proposal response failed", slog.String("operation", operation))
	}
	return proposal, nil
}

func (c *Client) do(ctx context.Context, operation, method, path string, body []byte) ([]byte, int, error) {
	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, bytes.NewReader(body))
	if err != nil {
		return nil, 0, domain.Error(err, "build pipeline request failed", slog.String("operation", operation))
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, 0, domain.Error(
			&domain.UpstreamUnreachableError{API: apiName, Cause: err},
			"pipeline request failed",
			slog.String("operation", operation),
		)
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, 0, domain.Error(
			&domain.UpstreamUnreachableError{API: apiName, Cause: err},
			"read pipeline response failed",
			slog.String("operation", operation),
		)
	}

	if resp.StatusCode >= http.StatusBadRequest {
		return nil, resp.StatusCode, domain.Error(
			&domain.UpstreamError{API: apiName, StatusCode: resp.StatusCode, Body: problemDetail(data)},
			"pipeline request rejected",
			slog.String("operation", operation),
		)
	}

	return data, resp.StatusCode, nil
}

func problemDetail(data []byte) string {
	var problem problemResponse
	if err := json.Unmarshal(data, &problem); err == nil && problem.Detail != "" {
		return problem.Detail
	}
	return string(data)
}
