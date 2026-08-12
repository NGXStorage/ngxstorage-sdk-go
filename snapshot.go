package ngxsdk

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
)

// SnapshotService manages point-in-time copies.
type SnapshotService struct{ c *Client }

// Snapshots returns the snapshot service.
func (c *Client) Snapshots() *SnapshotService { return &SnapshotService{c} }

// Create provisions a snapshot of a volume.
func (s *SnapshotService) Create(ctx context.Context, volumeID, name string) (*Snapshot, error) {
	body := map[string]interface{}{"name": name, "volume_id": volumeID}
	resp, err := s.c.mutation(ctx, "CreateSnapshot", nil, body)
	if err != nil {
		return nil, fmt.Errorf("ngx sdk: create snapshot: %w", err)
	}
	var snap Snapshot
	if err := json.Unmarshal(resp, &snap); err != nil {
		return nil, fmt.Errorf("ngx sdk: decode create snapshot: %w", err)
	}
	return &snap, nil
}

// Get fetches a single snapshot.
func (s *SnapshotService) Get(ctx context.Context, id string) (*Snapshot, error) {
	vars := url.Values{"id": {id}}
	body, err := s.c.get(ctx, "GetSnapshot", vars)
	if err != nil {
		return nil, fmt.Errorf("ngx sdk: get snapshot: %w", err)
	}
	var snap Snapshot
	if err := json.Unmarshal(body, &snap); err != nil {
		return nil, fmt.Errorf("ngx sdk: decode snapshot: %w", err)
	}
	return &snap, nil
}

// List fetches all snapshots.
func (s *SnapshotService) List(ctx context.Context) ([]Snapshot, error) {
	body, err := s.c.get(ctx, "GetSnapshotList", nil)
	if err != nil {
		return nil, fmt.Errorf("ngx sdk: list snapshots: %w", err)
	}
	var snaps []Snapshot
	if err := json.Unmarshal(body, &snaps); err != nil {
		return nil, fmt.Errorf("ngx sdk: decode snapshot list: %w", err)
	}
	return snaps, nil
}

// Delete removes a snapshot. Idempotent.
func (s *SnapshotService) Delete(ctx context.Context, id string) error {
	vars := url.Values{"id": {id}}
	_, err := s.c.mutation(ctx, "DeleteSnapshot", vars, nil)
	if IsNotFound(err) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("ngx sdk: delete snapshot: %w", err)
	}
	return nil
}

// Clone creates a new volume from a snapshot, returning the new volume ID.
func (s *SnapshotService) Clone(ctx context.Context, snapID, newName string) (string, error) {
	vars := url.Values{"id": {snapID}}
	body := map[string]interface{}{"name": newName}
	resp, err := s.c.mutation(ctx, "CloneSnapshot", vars, body)
	if err != nil {
		return "", fmt.Errorf("ngx sdk: clone snapshot: %w", err)
	}
	var parsed struct {
		VolumeID string `json:"volume_id"`
	}
	if err := json.Unmarshal(resp, &parsed); err != nil {
		return "", fmt.Errorf("ngx sdk: decode clone snapshot: %w", err)
	}
	if parsed.VolumeID == "" {
		return "", fmt.Errorf("ngx sdk: clone snapshot returned no volume ID")
	}
	return parsed.VolumeID, nil
}

// Restore reverts a volume to a snapshot.
func (s *SnapshotService) Restore(ctx context.Context, snapID string) error {
	vars := url.Values{"id": {snapID}}
	// The backend requires an empty body but a Content-Length header; an empty
	// JSON object satisfies both.
	body := map[string]interface{}{"confirm": "true"}
	_, err := s.c.mutation(ctx, "RestoreSnapshot", vars, body)
	if err != nil {
		return fmt.Errorf("ngx sdk: restore snapshot: %w", err)
	}
	return nil
}
