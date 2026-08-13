package ngxstorage

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
		return nil, fmt.Errorf("ngxstorage: create snapshot: %w", err)
	}
	var snap Snapshot
	if err := json.Unmarshal(resp, &snap); err != nil {
		return nil, fmt.Errorf("ngxstorage: decode create snapshot: %w", err)
	}
	return &snap, nil
}

// Get fetches a single snapshot.
func (s *SnapshotService) Get(ctx context.Context, id string) (*Snapshot, error) {
	vars := url.Values{"id": {id}}
	body, err := s.c.get(ctx, "GetSnapshot", vars)
	if err != nil {
		return nil, fmt.Errorf("ngxstorage: get snapshot: %w", err)
	}
	var snap Snapshot
	if err := json.Unmarshal(body, &snap); err != nil {
		return nil, fmt.Errorf("ngxstorage: decode snapshot: %w", err)
	}
	return &snap, nil
}

// List fetches all snapshots.
func (s *SnapshotService) List(ctx context.Context) ([]Snapshot, error) {
	body, err := s.c.get(ctx, "GetSnapshotList", nil)
	if err != nil {
		return nil, fmt.Errorf("ngxstorage: list snapshots: %w", err)
	}
	var snaps []Snapshot
	if err := json.Unmarshal(body, &snaps); err != nil {
		return nil, fmt.Errorf("ngxstorage: decode snapshot list: %w", err)
	}
	return snaps, nil
}

// ListDetail fetches detailed snapshots from /api/v2/snapshot/list, optionally
// filtered by one source volume ID. The NFS driver reads this endpoint for
// CSI ListSnapshots and source validation.
func (s *SnapshotService) ListDetail(ctx context.Context, volumeID string) ([]Snapshot, error) {
	vars := url.Values{}
	if volumeID != "" {
		// The backend filters snapshot/list by the canonical volume_id query
		// parameter.
		vars.Set("volume_id", volumeID)
	}
	body, err := s.c.get(ctx, "GetSnapshotDetailList", vars)
	if err != nil {
		return nil, fmt.Errorf("ngxstorage: list snapshot details: %w", err)
	}
	var snaps []Snapshot
	if err := json.Unmarshal(body, &snaps); err != nil {
		return nil, fmt.Errorf("ngxstorage: decode snapshot detail list: %w", err)
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
		return fmt.Errorf("ngxstorage: delete snapshot: %w", err)
	}
	return nil
}

// Clone creates a new volume from a snapshot, returning the new volume ID.
func (s *SnapshotService) Clone(ctx context.Context, snapID, newName string) (string, error) {
	vars := url.Values{"id": {snapID}}
	body := map[string]interface{}{"name": newName}
	resp, err := s.c.mutation(ctx, "CloneSnapshot", vars, body)
	if err != nil {
		return "", fmt.Errorf("ngxstorage: clone snapshot: %w", err)
	}
	var parsed struct {
		VolumeID string `json:"volume_id"`
	}
	if err := json.Unmarshal(resp, &parsed); err != nil {
		return "", fmt.Errorf("ngxstorage: decode clone snapshot: %w", err)
	}
	if parsed.VolumeID == "" {
		return "", fmt.Errorf("ngxstorage: clone snapshot returned no volume ID")
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
		return fmt.Errorf("ngxstorage: restore snapshot: %w", err)
	}
	return nil
}
