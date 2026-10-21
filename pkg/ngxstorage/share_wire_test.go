package ngxstorage

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
)

// Wire-contract tests for share operations.

func TestShareListNamesDecodesLightweightShape(t *testing.T) {
	client, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v2/share" {
			t.Fatalf("path = %q", r.URL.Path)
		}
		w.Write([]byte(`[{"id":"sh-1","name":"share1","owner":"ctrl-a","pool_name":"pool1"}]`))
	})
	refs, err := client.Shares().ListNames(context.Background())
	if err != nil {
		t.Fatalf("ListNames: %v", err)
	}
	if len(refs) != 1 || refs[0].ID != "sh-1" || refs[0].Name != "share1" {
		t.Fatalf("lightweight list mismatch: %+v", refs)
	}
}

func TestShareSetExportEnabledSendsNFSExport(t *testing.T) {
	var body map[string]interface{}
	client, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPatch || r.URL.Path != "/api/v2/share/sh-1" {
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.Path)
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode body: %v", err)
		}
		w.Write([]byte(`{"success":true}`))
	})
	if err := client.Shares().SetExportEnabled(context.Background(), "sh-1", true); err != nil {
		t.Fatalf("SetExportEnabled: %v", err)
	}
	if enabled, ok := body["nfs_export"].(bool); !ok || !enabled {
		t.Fatalf("nfs_export must be a JSON boolean: %+v", body)
	}
}

func TestShareSetReadOnlySendsNFSReadOnly(t *testing.T) {
	var body map[string]interface{}
	client, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode body: %v", err)
		}
		w.Write([]byte(`{"success":true}`))
	})
	if err := client.Shares().SetReadOnly(context.Background(), "sh-1", true); err != nil {
		t.Fatalf("SetReadOnly: %v", err)
	}
	if ro, ok := body["nfs_read_only"].(bool); !ok || !ro {
		t.Fatalf("nfs_read_only must be a JSON boolean: %+v", body)
	}
}

func TestShareExpandSendsFlatSoftQuota(t *testing.T) {
	var body map[string]interface{}
	client, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode body: %v", err)
		}
		w.Write([]byte(`{"success":true}`))
	})
	if err := client.Shares().Expand(context.Background(), "sh-1", 2<<30); err != nil {
		t.Fatalf("Expand: %v", err)
	}
	if quota, ok := body["soft_quota"].(float64); !ok || quota != 2<<30 {
		t.Fatalf("soft_quota must be a flat byte count: %+v", body)
	}
	if _, nested := body["capacity"]; nested {
		t.Fatalf("soft_quota must never be nested under capacity: %+v", body)
	}
}

func TestShareDeleteIdempotentOnSemanticNotFound(t *testing.T) {
	client, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"code":5011,"error":"no share"}`))
	})
	if err := client.Shares().Delete(context.Background(), "gone-1"); err != nil {
		t.Fatalf("Delete should be idempotent: %v", err)
	}
}
