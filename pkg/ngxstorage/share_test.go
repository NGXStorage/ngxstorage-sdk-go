package ngxstorage

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
)

// TestShareUnmarshalCanonicalFields verifies the Share struct decodes the
// live share detail wire shape: nested capacity quotas, nested storage
// attributes, and nested exports documents.
func TestShareUnmarshalCanonicalFields(t *testing.T) {
	raw := []byte(`{
		"id": "share-1",
		"name": "pvc-vol",
		"pool_name": "pool1",
		"owner": "V6BGWYPA_B",
		"created": 1756716344,
		"share_ips": ["192.0.2.1"],
		"capacity": {
			"used": "1024",
			"data": "512",
			"snapshots": "256",
			"hard_quota": null,
			"soft_quota": "1073741824",
			"reserved": null
		},
		"storage": {
			"blocksize": "128k",
			"deduplication": true,
			"compression": false,
			"provision_type": "thin",
			"dram_cache": true,
			"flash_cache": false,
			"io_type": "transactional",
			"permissions": "101101101",
			"flashtier_limit": "0"
		},
		"exports": {
			"nfs": {"enabled": true, "read_only": false, "recursive": false},
			"smb": {"enabled": false},
			"s3": {"enabled": false}
		}
	}`)
	var share Share
	if err := json.Unmarshal(raw, &share); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if share.ID != "share-1" || share.Name != "pvc-vol" || share.PoolName != "pool1" || share.Owner != "V6BGWYPA_B" {
		t.Fatalf("identity mismatch: %+v", share)
	}
	if share.Created == nil || int64(*share.Created) != 1756716344 {
		t.Fatalf("Created = %v, want 1756716344", share.Created)
	}
	if share.Capacity.SoftQuota == nil || int64(*share.Capacity.SoftQuota) != 1073741824 {
		t.Fatalf("SoftQuota = %v, want 1073741824", share.Capacity.SoftQuota)
	}
	if share.Capacity.HardQuota != nil || share.Capacity.Reserved != nil {
		t.Fatalf("null quotas must decode to nil: %+v", share.Capacity)
	}
	if int64(share.Capacity.Used) != 1024 {
		t.Fatalf("Used = %d, want 1024", int64(share.Capacity.Used))
	}
	if !share.Storage.Deduplication || share.Storage.Compression {
		t.Fatalf("storage dedup/compression mismatch: %+v", share.Storage)
	}
	if share.Storage.BlockSize != "128k" || share.Storage.Permissions != "101101101" || share.Storage.FlashTierLimit != "0" {
		t.Fatalf("storage attributes mismatch: %+v", share.Storage)
	}
	if !share.Exports.NFS.Enabled || share.Exports.NFS.ReadOnly {
		t.Fatalf("export state mismatch: %+v", share.Exports)
	}
}

// TestShareUnmarshalFlexibleEncodings covers alternate encodings: numeric
// soft_quota and string export booleans.
func TestShareUnmarshalFlexibleEncodings(t *testing.T) {
	raw := []byte(`{
		"id": "share-2",
		"name": "pvc-vol2",
		"pool_name": "pool1",
		"capacity": {"soft_quota": 5368709120},
		"exports": {"nfs": {"enabled": "1", "read_only": "0"}}
	}`)
	var share Share
	if err := json.Unmarshal(raw, &share); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if share.Capacity.SoftQuota == nil || int64(*share.Capacity.SoftQuota) != 5368709120 {
		t.Fatalf("SoftQuota = %v, want 5368709120", share.Capacity.SoftQuota)
	}
	if !share.Exports.NFS.Enabled {
		t.Fatalf("ExportEnabled = false, want true for \"1\"")
	}
	if share.Exports.NFS.ReadOnly {
		t.Fatalf("ReadOnly = true, want false for \"0\"")
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
	if share.Capacity.SoftQuota != nil {
		t.Fatalf("SoftQuota = %v, want nil for absent field", share.Capacity.SoftQuota)
	}
	if share.Exports.NFS.Enabled {
		t.Fatalf("ExportEnabled = true, want false for absent field")
	}
}

func TestShareUnmarshalKeepsMalformedListFieldsLocalToRecord(t *testing.T) {
	for _, raw := range []string{
		`{"id":1,"name":"pvc-vol","capacity":{"soft_quota":1073741824}}`,
		`{"id":"share-1","name":"pvc-vol","capacity":"invalid"}`,
		`{"id":"share-1","name":"pvc-vol","capacity":{"soft_quota":"invalid"}}`,
		`{"id":"share-1","name":"pvc-vol","storage":{"blocksize":1,"permissions":"111111111"}}`,
	} {
		var share Share
		if err := json.Unmarshal([]byte(raw), &share); err != nil {
			t.Fatalf("Unmarshal(%s): %v", raw, err)
		}
	}

	var share Share
	if err := json.Unmarshal([]byte(`{"id":"share-1","name":"pvc-vol","capacity":{"soft_quota":"invalid"},"storage":{"blocksize":1,"permissions":"111111111"}}`), &share); err != nil {
		t.Fatalf("Unmarshal mixed record: %v", err)
	}
	if share.ID != "share-1" || share.Capacity.SoftQuota != nil || share.Storage.BlockSize != "" || share.Storage.Permissions != "111111111" {
		t.Fatalf("malformed optional fields were not isolated: %+v", share)
	}
}

func TestShareUnmarshalRejectsMalformedExportState(t *testing.T) {
	for _, raw := range []string{
		`{"id":"share-1","exports":{"nfs":{"enabled":"maybe"}}}`,
		`{"id":"share-1","exports":{"nfs":{"read_only":{}}}}`,
	} {
		var share Share
		if err := json.Unmarshal([]byte(raw), &share); err == nil {
			t.Fatalf("Unmarshal(%s) accepted malformed export state", raw)
		}
	}
}

// TestShareServiceCreateResolvesID verifies Create sends only the fields the
// create route declares and resolves the created ID from the lightweight list.
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
			if _, ok := body["blocksize"]; !ok {
				t.Fatalf("create body must carry the explicit blocksize: %+v", body)
			}
			if _, ok := body["size"]; ok {
				t.Fatalf("create body must not use size: %+v", body)
			}
			if enabled, ok := body["nfs_export"].(bool); !ok || enabled {
				t.Fatalf("create body nfs_export must be false: %+v", body)
			}
			for _, undeclared := range []string{"flash_cache", "dram_cache", "flashtier_limit"} {
				if _, ok := body[undeclared]; ok {
					t.Fatalf("create body must not send undeclared field %s: %+v", undeclared, body)
				}
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
		BlockSize:      "128k",
		Dedup:          true,
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if created.ID != "share-9" {
		t.Fatalf("created ID = %q, want share-9", created.ID)
	}
	if created.Storage.Deduplication != FlexBool(true) {
		t.Fatalf("created storage deduplication = %v, want true", created.Storage.Deduplication)
	}
}

// TestShareServiceCreateOmitsEmptyBlocksize verifies the backend default
// blocksize (128k) applies when the caller does not choose one: an empty
// blocksize must not be sent, because the route rejects "" with 5008.
func TestShareServiceCreateOmitsEmptyBlocksize(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v2/share/":
			var body map[string]interface{}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Fatalf("decode create body: %v", err)
			}
			if _, ok := body["blocksize"]; ok {
				t.Fatalf("empty blocksize must be omitted from the create body: %+v", body)
			}
			w.Write([]byte(`{"success": true}`))
		case "/api/v2/share":
			w.Write([]byte(`[{"id": "share-10", "name": "pvc-default"}]`))
		default:
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
	})
	client, _ := newTestClient(t, handler)
	if _, err := client.Shares().Create(context.Background(), ShareCreateRequest{
		Name:           "pvc-default",
		SoftQuotaBytes: 1073741824,
	}); err != nil {
		t.Fatalf("Create: %v", err)
	}
}

// TestShareServiceCreateAppliesModifyOnlyAttributes verifies the cache and
// flashtier attributes requested at create time are applied with a follow-up
// modify using the canonical modify key names.
func TestShareServiceCreateAppliesModifyOnlyAttributes(t *testing.T) {
	flashCache, dramCache := true, true
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v2/share/":
			w.Write([]byte(`{"success": true}`))
		case "/api/v2/share":
			w.Write([]byte(`[{"id": "share-11", "name": "pvc-caches"}]`))
		case "/api/v2/share/share-11":
			var body map[string]interface{}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Fatalf("decode modify body: %v", err)
			}
			if body["flash_cache"] != true || body["dram_cache"] != true || body["flashtier_blocksize_limit"] != "16k" {
				t.Fatalf("modify body mismatch: %+v", body)
			}
			w.Write([]byte(`{"success": true}`))
		default:
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
	})
	client, _ := newTestClient(t, handler)
	created, err := client.Shares().Create(context.Background(), ShareCreateRequest{
		Name:           "pvc-caches",
		SoftQuotaBytes: 1073741824,
		FlashCache:     &flashCache,
		DramCache:      &dramCache,
		FlashTierLimit: "16k",
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if !created.Storage.FlashCache || !created.Storage.DramCache || created.Storage.FlashTierLimit != "16k" {
		t.Fatalf("created share attributes mismatch: %+v", created.Storage)
	}
}

func TestShareServiceCreateAppliesExplicitDisabledCacheAttributes(t *testing.T) {
	flashCache, dramCache := false, false
	modifyCalls := 0
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v2/share/":
			w.Write([]byte(`{"success":true}`))
		case "/api/v2/share":
			w.Write([]byte(`[{"id":"share-12","name":"pvc-no-cache"}]`))
		case "/api/v2/share/share-12":
			modifyCalls++
			var body map[string]interface{}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Fatalf("decode modify body: %v", err)
			}
			if body["flash_cache"] != false || body["dram_cache"] != false {
				t.Fatalf("modify body must preserve explicit disabled values: %+v", body)
			}
			w.Write([]byte(`{"success":true}`))
		default:
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
	})
	client, _ := newTestClient(t, handler)
	created, err := client.Shares().Create(context.Background(), ShareCreateRequest{
		Name:           "pvc-no-cache",
		SoftQuotaBytes: 1073741824,
		FlashCache:     &flashCache,
		DramCache:      &dramCache,
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if modifyCalls != 1 || created.Storage.FlashCache || created.Storage.DramCache {
		t.Fatalf("create did not preserve requested cache state: modifies=%d storage=%+v", modifyCalls, created.Storage)
	}
}
