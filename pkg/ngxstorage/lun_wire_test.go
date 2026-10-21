package ngxstorage

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
)

// Wire-contract tests for LUN operations: request bodies and response shapes
// are pinned to the backend contract.

const wireLUNDetail = `{
  "id": "7176302add549ceaf03caef021707ca0e6021f89",
  "name": "probe-lun",
  "owner": "ctrl-a",
  "pool_name": "pool1",
  "size": "1073741824",
  "used": "1048576",
  "blocksize": "32k",
  "thin_provision": true,
  "compression": true,
  "deduplication": false,
  "dram_cache": true,
  "flash_cache": false,
  "compress_ratio": "1.01",
  "clone": "",
  "sizebysnapshots": "0",
  "scsi_id": "3600507680012345678",
  "io_type": "transactional",
  "qos_priority": 4,
  "protocols": {"fc": null, "iscsi": null},
  "created": "2025-11-24T08:50:44Z"
}`

func TestLUNListDecodesWireShape(t *testing.T) {
	client, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v2/lun/list" {
			t.Fatalf("path = %q", r.URL.Path)
		}
		w.Write([]byte("[" + wireLUNDetail + "]"))
	})
	luns, err := client.LUNs().List(context.Background())
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(luns) != 1 {
		t.Fatalf("luns = %d, want 1", len(luns))
	}
	lun := luns[0]
	if lun.ID != "7176302add549ceaf03caef021707ca0e6021f89" || lun.Name != "probe-lun" || lun.PoolName != "pool1" {
		t.Fatalf("identity mismatch: %+v", lun)
	}
	if int64(lun.Size) != 1073741824 || int64(lun.Used) != 1048576 {
		t.Fatalf("size/used = %d/%d", int64(lun.Size), int64(lun.Used))
	}
	if lun.ThinProvision != "on" || lun.Compression != "on" || lun.Deduplication != "off" || lun.DramCache != "on" || lun.FlashCache != "off" {
		t.Fatalf("flag decoding wrong: %+v", lun)
	}
	if lun.Protocols["fc"] != "" || lun.Protocols["iscsi"] != "" {
		t.Fatalf("null protocol refs must decode empty: %+v", lun.Protocols)
	}
}

func TestLUNGetReturnsSingleRecord(t *testing.T) {
	client, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v2/lun/lun-1" {
			t.Fatalf("path = %q", r.URL.Path)
		}
		w.Write([]byte(wireLUNDetail))
	})
	lun, err := client.LUNs().Get(context.Background(), "lun-1")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if lun.Name != "probe-lun" || int64(lun.QosPriority) != 4 {
		t.Fatalf("decode mismatch: %+v", lun)
	}
}

func TestLUNCreateWithOptionsSendsOnlySetFields(t *testing.T) {
	client, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/api/v2/lun/" {
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.Path)
		}
		var body map[string]interface{}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode body: %v", err)
		}
		// Backend defaults apply for anything omitted; set fields are sent.
		if body["blocksize"] != "32k" || body["deduplication"] != "on" || body["qos_priority"] != float64(4) {
			t.Fatalf("set create fields must be sent: %+v", body)
		}
		for _, absent := range []string{"compression", "thin_provision", "io_type", "quantity"} {
			if _, ok := body[absent]; ok {
				t.Fatalf("zero-value field %s must be omitted: %+v", absent, body)
			}
		}
		if body["pool_name"] != "pool1" || body["name"] != "vol1" || body["size"] != float64(100) || body["sing"] != "G" {
			t.Fatalf("required create keys wrong: %+v", body)
		}
		w.Write([]byte(`{"success":true,"luns":[{"id":"lun-1","name":"vol1","scsi_id":"s1","used":"0"}]}`))
	})
	lun, err := client.LUNs().CreateWithOptions(context.Background(), "vol1", 100, LUNCreateOptions{
		Blocksize:     "32k",
		Deduplication: "on",
		QosPriority:   4,
	})
	if err != nil {
		t.Fatalf("CreateWithOptions: %v", err)
	}
	if lun.ID != "lun-1" {
		t.Fatalf("lun ID = %q", lun.ID)
	}
}

func TestLUNModifySendsBodyAndParsesAck(t *testing.T) {
	var gotBody map[string]interface{}
	client, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPatch || r.URL.Path != "/api/v2/lun/lun-1" {
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.Path)
		}
		if err := json.NewDecoder(r.Body).Decode(&gotBody); err != nil {
			t.Fatalf("decode body: %v", err)
		}
		w.Write([]byte(`{"success":true,"size":"2147483648"}`))
	})
	if err := client.LUNs().Modify(context.Background(), "lun-1", map[string]interface{}{
		"deduplication": "off", "force": "true",
	}); err != nil {
		t.Fatalf("Modify: %v", err)
	}
	if gotBody["deduplication"] != "off" || gotBody["force"] != "true" {
		t.Fatalf("modify body must pass fields verbatim: %+v", gotBody)
	}
}

func TestLUNDeleteIdempotentOnSemanticNotFound(t *testing.T) {
	calls := 0
	client, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.Write([]byte(`{"code":5011,"error":"no lun"}`))
	})
	if err := client.LUNs().Delete(context.Background(), "gone-1"); err != nil {
		t.Fatalf("Delete should be idempotent: %v", err)
	}
	if calls != 1 {
		t.Fatalf("calls = %d, want 1", calls)
	}
}
