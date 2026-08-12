package ngxsdk

import (
	"context"
	"encoding/json"
	"fmt"
)

// NetworkService queries network information.
type NetworkService struct{ c *Client }

// Network returns the network service.
func (c *Client) Network() *NetworkService { return &NetworkService{c} }

// Info returns the network info document.
func (s *NetworkService) Info(ctx context.Context) (map[string]interface{}, error) {
	body, err := s.c.get(ctx, "GetNetworkInfo", nil)
	if err != nil {
		return nil, fmt.Errorf("ngx sdk: get network info: %w", err)
	}
	var m map[string]interface{}
	if err := json.Unmarshal(body, &m); err != nil {
		return nil, fmt.Errorf("ngx sdk: decode network info: %w", err)
	}
	return m, nil
}

// InfoService queries system information.
type InfoService struct{ c *Client }

// Info returns the info service.
func (c *Client) Info() *InfoService { return &InfoService{c} }

// System returns the system info document.
func (s *InfoService) System(ctx context.Context) (map[string]interface{}, error) {
	body, err := s.c.get(ctx, "GetSystemInfo", nil)
	if err != nil {
		return nil, fmt.Errorf("ngx sdk: get system info: %w", err)
	}
	var m map[string]interface{}
	if err := json.Unmarshal(body, &m); err != nil {
		return nil, fmt.Errorf("ngx sdk: decode system info: %w", err)
	}
	return m, nil
}
