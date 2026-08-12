package ngxsdk

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
		return nil, fmt.Errorf("ngx sdk: list hosts: %w", err)
	}
	var m []map[string]interface{}
	if err := json.Unmarshal(body, &m); err != nil {
		return nil, fmt.Errorf("ngx sdk: decode host list: %w", err)
	}
	return m, nil
}

// Get fetches a single host.
func (s *HostService) Get(ctx context.Context, id string) (map[string]interface{}, error) {
	vars := url.Values{"id": {id}}
	body, err := s.c.get(ctx, "GetHost", vars)
	if err != nil {
		return nil, fmt.Errorf("ngx sdk: get host: %w", err)
	}
	var m map[string]interface{}
	if err := json.Unmarshal(body, &m); err != nil {
		return nil, fmt.Errorf("ngx sdk: decode host: %w", err)
	}
	return m, nil
}
