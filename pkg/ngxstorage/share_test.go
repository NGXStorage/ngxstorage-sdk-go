package ngxstorage

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
)

// TestShareUnmarshalCanonicalFields verifies the Share struct decodes the
// canonical NFS backend field shapes: nested capacity.soft_quota (number or
// string), nested exports.nfs.enabled ("1"/"" or boolean), read_only, and the
// canonical storage attribute names.
func TestShareUnmarshalCanonicalFields(t *testing.T) {
	raw := []byte(`{
		"id": "share-1",
		"name": "pvc-vol",
		"pool_name": "pool1",
		"capacity": {"soft_quota": "1073741824"},
		"exports": {"nfs": {"enabled": "1", "read_only": ""}},
		"blocksize": "4k",
		"deduplication": "on",
		"compression": "off",
		"flash_cache": "1",
		"dram_cache": "",
		"permissions": "0770",
		"flashtier_limit": "50"
	}`)
	var share Share
	if err := json.Unmarshal(raw, &share); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if share.ID != "share-1" || share.Name != "pvc-vol" || share.PoolName != "pool1" {
		t.Fatalf("identity mismatch: %+v", share)
	}
	if int64(share.Capacity.SoftQuota) != 1073741824 {
		t.Fatalf("SoftQuota = %d, want 1073741824", int64(share.Capacity.SoftQuota))
	}
	if !share.Exports.NFS.Enabled {
		t.Fatalf("ExportEnabled = false, want true for \"1\"")
	}
	if share.Exports.NFS.ReadOnly {
		t.Fatalf("ReadOnly = true, want false for \"\"")
	}
	if share.BlockSize != "4k" || share.Dedup != "on" || share.Compress != "off" ||
		share.FlashCache != "1" || share.DramCache != "" || share.Permissions != "0770" ||
		share.FlashTierLimit != "50" {
		t.Fatalf("storage attributes mismatch: %+v", share)
	}
}

// TestShareUnmarshalFlexibleEncodings covers alternate documented encodings:
// numeric soft_quota, boolean enabled, and string read_only.
func TestShareUnmarshalFlexibleEncodings(t *testing.T) {
	raw := []byte(`{
		"id": "share-2",
		"name": "pvc-vol2",
		"pool_name": "pool1",
		"capacity": {"soft_quota": 5368709120},
		"exports": {"nfs": {"enabled": true, "read_only": "1"}}
	}`)
	var share Share
	if err := json.Unmarshal(raw, &share); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if int64(share.Capacity.SoftQuota) != 5368709120 {
		t.Fatalf("SoftQuota = %d, want 5368709120", int64(share.Capacity.SoftQuota))
	}
	if !share.Exports.NFS.Enabled {
		t.Fatalf("ExportEnabled = false, want true for boolean true")
	}
	if !share.Exports.NFS.ReadOnly {
		t.Fatalf("ReadOnly = false, want true for \"1\"")
	}
}

// TestShareUnmarshalMissingFields verifies absent canonical documents decode
// to zero values rather than failing.
func TestShareUnmarshalMissingFields(t *testing.T) {
	raw := []byte(`{"id": "share-3", "name": "pvc-vol3", "pool_name": "pool1"}`)
	var share Share
	if err := json.Unmarshal(raw, &share); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if int64(share.Capacity.SoftQuota) != 0 {
		t.Fatalf("SoftQuota = %d, want 0", int64(share.Capacity.SoftQuota))
	}
	if share.Exports.NFS.Enabled {
		t.Fatalf("ExportEnabled = true, want false for absent field")
	}
}

// TestShareServiceCreateResolvesID verifies Create sends the canonical flat
// soft_quota field and resolves the created ID from the lightweight list.
func TestShareServiceCreateResolvesID(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v2/share/":
			// Assert the canonical create payload.
			var body map[string]interface{}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Fatalf("decode create body: %v", err)
			}
			if _, ok := body["soft_quota"]; !ok {
				t.Fatalf("create body missing canonical soft_quota: %+v", body)
			}
			if _, ok := body["size"]; ok {
				t.Fatalf("create body must not use size: %+v", body)
			}
			if enabled, ok := body["nfs_export"].(bool); !ok || enabled {
				t.Fatalf("create body nfs_export must be false: %+v", body)
			}
			w.Write([]byte(`{"success": true}`))
		case "/api/v2/share":
			w.Write([]byte(`[{"id": "share-9", "name": "pvc-new"}]`))
		default:
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
	})
	client, _ := newTestClient(t, handler)
	created, err := client.Shares().Create(context.Background(), ShareCreateRequest{
		Name:           "pvc-new",
		SoftQuotaBytes: 1073741824,
		BlockSize:      "4k",
		Dedup:          true,
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if created.ID != "share-9" {
		t.Fatalf("created ID = %q, want share-9", created.ID)
	}
}
