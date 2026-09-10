package ngxstorage

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
)

// HostService lists hosts.
type HostService struct{ c *Client }

// Hosts returns the host service.
func (c *Client) Hosts() *HostService { return &HostService{c} }

// List fetches all hosts.
func (s *HostService) List(ctx context.Context) ([]map[string]interface{}, error) {
	body, err := s.c.get(ctx, "GetHostList", nil)
	if err != nil {
		return nil, fmt.Errorf("ngxstorage: list hosts: %w", err)
	}
	var m []map[string]interface{}
	if err := json.Unmarshal(body, &m); err != nil {
		return nil, fmt.Errorf("ngxstorage: decode host list: %w", err)
	}
	return m, nil
}

// Get fetches a single host.
func (s *HostService) Get(ctx context.Context, id string) (map[string]interface{}, error) {
	vars := url.Values{"id": {id}}
	body, err := s.c.get(ctx, "GetHost", vars)
	if err != nil {
		return nil, fmt.Errorf("ngxstorage: get host: %w", err)
	}
	var m map[string]interface{}
	if err := json.Unmarshal(body, &m); err != nil {
		return nil, fmt.Errorf("ngxstorage: decode host: %w", err)
	}
	return m, nil
}

// ListDetail fetches all hosts with full detail from /api/v2/host/list.
func (s *HostService) ListDetail(ctx context.Context) ([]map[string]interface{}, error) {
	body, err := s.c.get(ctx, "GetHostDetailList", nil)
	if err != nil {
		return nil, fmt.Errorf("ngxstorage: list host details: %w", err)
	}
	var m []map[string]interface{}
	if err := json.Unmarshal(body, &m); err != nil {
		return nil, fmt.Errorf("ngxstorage: decode host detail list: %w", err)
	}
	return m, nil
}

// Create provisions a host.
func (s *HostService) Create(ctx context.Context, host map[string]interface{}) (string, error) {
	resp, err := s.c.mutation(ctx, "CreateHost", nil, host)
	if err != nil {
		return "", fmt.Errorf("ngxstorage: create host: %w", err)
	}
	var parsed struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(resp, &parsed); err != nil {
		return "", fmt.Errorf("ngxstorage: decode create host: %w", err)
	}
	return parsed.ID, nil
}

// Delete removes a host. Idempotent.
func (s *HostService) Delete(ctx context.Context, id string) error {
	vars := url.Values{"id": {id}}
	_, err := s.c.mutation(ctx, "DeleteHost", vars, nil)
	if IsNotFound(err) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("ngxstorage: delete host: %w", err)
	}
	return nil
}

// Modify updates a host.
func (s *HostService) Modify(ctx context.Context, id string, host map[string]interface{}) error {
	vars := url.Values{"id": {id}}
	_, err := s.c.mutation(ctx, "ModifyHost", vars, host)
	if err != nil {
		return fmt.Errorf("ngxstorage: modify host: %w", err)
	}
	return nil
}
