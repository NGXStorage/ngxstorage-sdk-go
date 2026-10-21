package ngxstorage

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

// Wire-contract tests for snapshot operations.

func TestSnapshotCreateDecodesNormalizedAck(t *testing.T) {
	var body map[string]interface{}
	client, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/api/v2/snapshot" {
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.Path)
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode body: %v", err)
		}
		// The create ack is the normalized snapshot document: it carries
		// id, path, name, volume_id, volume_name, and pool_name.
		w.Write([]byte(`{
			"id": "08282136ef7e8708ee8c29c3ee88cd820da2037e",
			"path": "pool1/vol1@snap1",
			"name": "snap1",
			"volume_id": "vol-1",
			"volume_name": "vol1",
			"pool_name": "pool1",
			"created": "2026-09-10 12:00:00",
			"reserved": "1048576",
			"binded": 0,
			"restore": 1,
			"vol_size": "1073741824"
		}`))
	})
	snap, err := client.Snapshots().Create(context.Background(), "vol-1", "snap1")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if body["name"] != "snap1" || body["volume_id"] != "vol-1" {
		t.Fatalf("create body mismatch: %+v", body)
	}
	if snap.ID != "08282136ef7e8708ee8c29c3ee88cd820da2037e" || snap.Path != "pool1/vol1@snap1" || snap.VolumeID != "vol-1" {
		t.Fatalf("normalized ack decoding mismatch: %+v", snap)
	}
	if !snap.Ready {
		t.Fatal("synchronous backend snapshot must decode Ready=true")
	}
}

func TestSnapshotListDetailSendsVolumeIDQueryFilter(t *testing.T) {
	client, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v2/snapshot/list" {
			t.Fatalf("path = %q", r.URL.Path)
		}
		if got := r.URL.Query().Get("volume_id"); got != "vol-1" {
			t.Fatalf("volume_id query = %q", got)
		}
		w.Write([]byte(`[]`))
	})
	if _, err := client.Snapshots().ListDetail(context.Background(), "vol-1"); err != nil {
		t.Fatalf("ListDetail: %v", err)
	}
}

func TestSnapshotRestoreSendsConfirm(t *testing.T) {
	var body map[string]interface{}
	client, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/api/v2/snapshot/restore/snap-1" {
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.Path)
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode body: %v", err)
		}
		w.Write([]byte(`{"success":true}`))
	})
	if err := client.Snapshots().Restore(context.Background(), "snap-1"); err != nil {
		t.Fatalf("Restore: %v", err)
	}
	if body["confirm"] != "true" {
		t.Fatalf("restore must send confirm=true: %+v", body)
	}
}

func TestSnapshotDeleteAndDeleteAllAreIdempotent(t *testing.T) {
	client, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/snapshot") && !strings.Contains(r.URL.Path, "/snapshot/") {
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
		w.Write([]byte(`{"code":5011,"error":"no snapshot"}`))
	})
	if err := client.Snapshots().Delete(context.Background(), "snap-gone"); err != nil {
		t.Fatalf("Delete should be idempotent: %v", err)
	}
	if err := client.Snapshots().DeleteAll(context.Background(), "vol-gone"); err != nil {
		t.Fatalf("DeleteAll should be idempotent: %v", err)
	}
}

func TestSnapshotDeleteScheduleIsIdempotent(t *testing.T) {
	client, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete || r.URL.Path != "/api/v2/snapshot/schedule/vol-1" {
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.Path)
		}
		w.Write([]byte(`{"code":5011,"error":"no schedule"}`))
	})
	if err := client.Snapshots().DeleteSchedule(context.Background(), "vol-1"); err != nil {
		t.Fatalf("DeleteSchedule should be idempotent: %v", err)
	}
}

func TestSnapshotCloneReadsVolumeIDAck(t *testing.T) {
	client, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/api/v2/snapshot/clone/snap-1" {
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.Path)
		}
		var body map[string]interface{}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode body: %v", err)
		}
		if body["name"] != "clone-1" {
			t.Fatalf("clone body mismatch: %+v", body)
		}
		// The clone ack carries the new volume's canonical volume_id.
		w.Write([]byte(`{"success":true,"volume_id":"new-vol-1"}`))
	})
	volID, err := client.Snapshots().Clone(context.Background(), "snap-1", "clone-1")
	if err != nil {
		t.Fatalf("Clone: %v", err)
	}
	if volID != "new-vol-1" {
		t.Fatalf("volID = %q", volID)
	}
}
