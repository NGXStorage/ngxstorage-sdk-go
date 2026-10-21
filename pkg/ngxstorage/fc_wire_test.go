package ngxstorage

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
)

// Wire-contract tests for FC target LUN mapping operations.

func TestFCTargetAddLUNSendsOwnerAndLunIDs(t *testing.T) {
	var body map[string]interface{}
	client, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut || r.URL.Path != "/api/v2/target/fc/lun/tgt-1" {
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.Path)
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode body: %v", err)
		}
		// The ack re-lists the mapped LUNs with integer SCSI numbers.
		w.Write([]byte(`{"success":true,"luns":[{"id":"lun-1","name":"vol1","number":6,"pool_name":"pool1","scsi_id":"3600"}]}`))
	})
	if err := client.FCTargets().AddLUN(context.Background(), "tgt-1", "lun-1", "ctrl-a"); err != nil {
		t.Fatalf("AddLUN: %v", err)
	}
	luns, ok := body["lun_ids"].([]interface{})
	if !ok || len(luns) != 1 || luns[0] != "lun-1" || body["owner"] != "ctrl-a" {
		t.Fatalf("add-lun body mismatch: %+v", body)
	}
}

func TestFCTargetRemoveLUNSendsOwnerAndLunIDs(t *testing.T) {
	var body map[string]interface{}
	client, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPatch || r.URL.Path != "/api/v2/target/fc/lun/tgt-1" {
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.Path)
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode body: %v", err)
		}
		w.Write([]byte(`{"success":true,"luns":[]}`))
	})
	if err := client.FCTargets().RemoveLUN(context.Background(), "tgt-1", "lun-1", "ctrl-a"); err != nil {
		t.Fatalf("RemoveLUN: %v", err)
	}
	luns, ok := body["lun_ids"].([]interface{})
	if !ok || len(luns) != 1 || luns[0] != "lun-1" || body["owner"] != "ctrl-a" {
		t.Fatalf("remove-lun body mismatch: %+v", body)
	}
}

func TestFCTargetAddLUNTreatsDuplicateAsSuccess(t *testing.T) {
	client, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"code":710,"error":"LUN already mapped"}`))
	})
	if err := client.FCTargets().AddLUN(context.Background(), "tgt-1", "lun-1", "ctrl-a"); err != nil {
		t.Fatalf("already-mapped LUN must be treated as success: %v", err)
	}
}
