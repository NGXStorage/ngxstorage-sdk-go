package ngxsdk

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
)

// LUNService manages block volumes (LUNs).
type LUNService struct{ c *Client }

// LUNs returns the LUN service.
func (c *Client) LUNs() *LUNService { return &LUNService{c} }

// Create provisions a LUN on the configured pool.
func (s *LUNService) Create(ctx context.Context, name string, sizeGB int64, owner string) (*LUN, error) {
	body := map[string]interface{}{
		"pool_name": s.c.poolName,
		"name":      name,
		"size":      sizeGB,
		"sing":      "G",
		"owner":     owner,
	}
	resp, err := s.c.mutation(ctx, "CreateLUN", nil, body)
	if err != nil {
		return nil, fmt.Errorf("ngx sdk: create LUN: %w", err)
	}
	var parsed struct {
		Luns []LUN `json:"luns"`
	}
	if err := json.Unmarshal(resp, &parsed); err != nil {
		return nil, fmt.Errorf("ngx sdk: decode create LUN: %w", err)
	}
	if len(parsed.Luns) == 0 {
		return nil, fmt.Errorf("ngx sdk: create LUN returned no LUN")
	}
	return &parsed.Luns[0], nil
}

// Get fetches a single LUN.
func (s *LUNService) Get(ctx context.Context, id string) (*LUN, error) {
	vars := url.Values{"id": {id}}
	body, err := s.c.get(ctx, "GetLUN", vars)
	if err != nil {
		return nil, fmt.Errorf("ngx sdk: get LUN: %w", err)
	}
	var lun LUN
	if err := json.Unmarshal(body, &lun); err != nil {
		return nil, fmt.Errorf("ngx sdk: decode LUN: %w", err)
	}
	return &lun, nil
}

// List fetches all LUNs.
func (s *LUNService) List(ctx context.Context) ([]LUN, error) {
	body, err := s.c.get(ctx, "GetLUNDetailList", nil)
	if err != nil {
		return nil, fmt.Errorf("ngx sdk: list LUNs: %w", err)
	}
	var luns []LUN
	if err := json.Unmarshal(body, &luns); err != nil {
		return nil, fmt.Errorf("ngx sdk: decode LUN list: %w", err)
	}
	return luns, nil
}

// Delete removes a LUN. Idempotent: a not-found backend code is success.
func (s *LUNService) Delete(ctx context.Context, id string) error {
	vars := url.Values{"id": {id}}
	_, err := s.c.mutation(ctx, "DeleteLUN", vars, nil)
	if IsNotFound(err) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("ngx sdk: delete LUN: %w", err)
	}
	return nil
}

// Modify patches arbitrary supported LUN fields.
func (s *LUNService) Modify(ctx context.Context, id string, fields map[string]interface{}) error {
	vars := url.Values{"id": {id}}
	_, err := s.c.mutation(ctx, "ModifyLUN", vars, fields)
	if err != nil {
		return fmt.Errorf("ngx sdk: modify LUN: %w", err)
	}
	return nil
}

// Expand grows a LUN to sizeGB (gigabytes).
func (s *LUNService) Expand(ctx context.Context, id string, sizeGB int64) error {
	return s.Modify(ctx, id, map[string]interface{}{"size": sizeGB, "sing": "G"})
}
