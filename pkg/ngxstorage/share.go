package ngxstorage

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
)

// ShareService manages file (NFS) volumes.
type ShareService struct{ c *Client }

// Shares returns the share service.
func (c *Client) Shares() *ShareService { return &ShareService{c} }

// Create provisions an NFS share on the configured pool.
func (s *ShareService) Create(ctx context.Context, name string, sizeBytes int64, owner string) (*Share, error) {
	body := map[string]interface{}{
		"pool_name": s.c.poolName,
		"name":      name,
		"size":      sizeBytes,
		"owner":     owner,
	}
	resp, err := s.c.mutation(ctx, "CreateShare", nil, body)
	if err != nil {
		return nil, fmt.Errorf("ngxstorage: create share: %w", err)
	}
	var parsed struct {
		Shares []Share `json:"shares"`
	}
	if err := json.Unmarshal(resp, &parsed); err != nil {
		return nil, fmt.Errorf("ngxstorage: decode create share: %w", err)
	}
	if len(parsed.Shares) == 0 {
		return nil, fmt.Errorf("ngxstorage: create share returned no share")
	}
	return &parsed.Shares[0], nil
}

// Get fetches a single share.
func (s *ShareService) Get(ctx context.Context, id string) (*Share, error) {
	vars := url.Values{"id": {id}}
	body, err := s.c.get(ctx, "GetShare", vars)
	if err != nil {
		return nil, fmt.Errorf("ngxstorage: get share: %w", err)
	}
	var share Share
	if err := json.Unmarshal(body, &share); err != nil {
		return nil, fmt.Errorf("ngxstorage: decode share: %w", err)
	}
	return &share, nil
}

// List fetches all shares.
func (s *ShareService) List(ctx context.Context) ([]Share, error) {
	body, err := s.c.get(ctx, "GetShareDetailList", nil)
	if err != nil {
		return nil, fmt.Errorf("ngxstorage: list shares: %w", err)
	}
	var shares []Share
	if err := json.Unmarshal(body, &shares); err != nil {
		return nil, fmt.Errorf("ngxstorage: decode share list: %w", err)
	}
	return shares, nil
}

// Delete removes a share. Idempotent.
func (s *ShareService) Delete(ctx context.Context, id string) error {
	vars := url.Values{"id": {id}}
	_, err := s.c.mutation(ctx, "DeleteShare", vars, nil)
	if IsNotFound(err) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("ngxstorage: delete share: %w", err)
	}
	return nil
}

// Modify patches arbitrary supported share fields.
func (s *ShareService) Modify(ctx context.Context, id string, fields map[string]interface{}) error {
	vars := url.Values{"id": {id}}
	_, err := s.c.mutation(ctx, "ModifyShare", vars, fields)
	if err != nil {
		return fmt.Errorf("ngxstorage: modify share: %w", err)
	}
	return nil
}

// Expand grows a share to sizeBytes.
func (s *ShareService) Expand(ctx context.Context, id string, sizeBytes int64) error {
	return s.Modify(ctx, id, map[string]interface{}{"size": sizeBytes})
}

// SetExportEnabled enables or disables the NFS export for a share.
func (s *ShareService) SetExportEnabled(ctx context.Context, id string, enabled bool) error {
	return s.Modify(ctx, id, map[string]interface{}{"nfs_export": enabled})
}

// SetReadOnly sets the read-only state of a share export.
func (s *ShareService) SetReadOnly(ctx context.Context, id string, readOnly bool) error {
	return s.Modify(ctx, id, map[string]interface{}{"read_only": readOnly})
}
