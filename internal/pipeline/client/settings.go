package client

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"

	"claude-memory-sync/internal/domain"
	"claude-memory-sync/internal/settings"
)

type PutSettingRequest struct {
	Value string `json:"value"`
	Actor string `json:"actor"`
}

type listSettingsResponse struct {
	Settings []settings.Setting `json:"settings"`
}

type settingHistoryResponse struct {
	Versions []settings.Version `json:"versions"`
}

func settingPath(key, suffix string) string {
	return "/v1/settings/" + url.PathEscape(key) + suffix
}

func (c *Client) ListSettings(ctx context.Context) ([]settings.Setting, error) {
	data, _, err := c.do(ctx, "list_settings", http.MethodGet, "/v1/settings", nil)
	if err != nil {
		return nil, err
	}

	var resp listSettingsResponse
	if err := json.Unmarshal(data, &resp); err != nil {
		return nil, domain.Error(err, "decode list settings response failed", slog.String("operation", "list_settings"))
	}
	if resp.Settings == nil {
		return []settings.Setting{}, nil
	}
	return resp.Settings, nil
}

func (c *Client) GetSetting(ctx context.Context, key string) (settings.Setting, error) {
	data, _, err := c.do(ctx, "get_setting", http.MethodGet, settingPath(key, ""), nil)
	if err != nil {
		return settings.Setting{}, err
	}

	var setting settings.Setting
	if err := json.Unmarshal(data, &setting); err != nil {
		return settings.Setting{}, domain.Error(err, "decode get setting response failed", slog.String("operation", "get_setting"))
	}
	return setting, nil
}

func (c *Client) PutSetting(ctx context.Context, key string, req PutSettingRequest) (settings.Setting, error) {
	body, err := json.Marshal(req)
	if err != nil {
		return settings.Setting{}, domain.Error(err, "encode put setting request failed")
	}

	data, _, err := c.do(ctx, "put_setting", http.MethodPut, settingPath(key, ""), body)
	if err != nil {
		return settings.Setting{}, err
	}

	var setting settings.Setting
	if err := json.Unmarshal(data, &setting); err != nil {
		return settings.Setting{}, domain.Error(err, "decode put setting response failed", slog.String("operation", "put_setting"))
	}
	return setting, nil
}

func (c *Client) DeleteSetting(ctx context.Context, key, actor string) error {
	path := settingPath(key, "") + "?" + url.Values{"actor": {actor}}.Encode()
	_, _, err := c.do(ctx, "delete_setting", http.MethodDelete, path, nil)
	return err
}

func (c *Client) SettingHistory(ctx context.Context, key string, limit int) ([]settings.Version, error) {
	path := settingPath(key, "/history")
	if limit > 0 {
		path += "?" + url.Values{"limit": {strconv.Itoa(limit)}}.Encode()
	}

	data, _, err := c.do(ctx, "setting_history", http.MethodGet, path, nil)
	if err != nil {
		return nil, err
	}

	var resp settingHistoryResponse
	if err := json.Unmarshal(data, &resp); err != nil {
		return nil, domain.Error(err, "decode setting history response failed", slog.String("operation", "setting_history"))
	}
	if resp.Versions == nil {
		return []settings.Version{}, nil
	}
	return resp.Versions, nil
}
