package ngxstorage

import (
	"context"
	"net/http"
	"testing"
)

// Wire-contract tests for pool and status operations.

func TestPoolListDecodesLightweightShape(t *testing.T) {
	client, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v2/pool" {
			t.Fatalf("path = %q", r.URL.Path)
		}
		w.Write([]byte(`[{"id":"12","name":"pool1","owner":"ctrl-a","is_flash":1}]`))
	})
	pools, err := client.Pools().List(context.Background())
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(pools) != 1 || pools[0].ID != "12" || pools[0].Name != "pool1" || !pools[0].IsFlash {
		t.Fatalf("lightweight pool mismatch: %+v", pools)
	}
}

func TestPoolListDetailDecodesOperationalShape(t *testing.T) {
	client, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v2/pool/list" {
			t.Fatalf("path = %q", r.URL.Path)
		}
		w.Write([]byte(`[{"id":"12","name":"pool1","owner":"ctrl-a","blocksize":"32k","is_flash":true,"raidtype":"RAID-Z2","logical":{"available":"100","reserved":"10"},"physical":{"allocated":"200","available":"50","total":"300"},"compress_ratio":"1.0","deduplication_ratio":"1.1"}]`))
	})
	pools, err := client.Pools().ListDetail(context.Background())
	if err != nil {
		t.Fatalf("ListDetail: %v", err)
	}
	if len(pools) != 1 || pools[0].BlockSize != "32k" || !pools[0].IsFlash {
		t.Fatalf("detail pool mismatch: %+v", pools)
	}
}

func TestPoolGetReturnsSinglePool(t *testing.T) {
	client, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v2/pool/12" {
			t.Fatalf("path = %q", r.URL.Path)
		}
		w.Write([]byte(`{"id":"12","name":"pool1","owner":"ctrl-a"}`))
	})
	pool, err := client.Pools().Get(context.Background(), "12")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if pool.Name != "pool1" {
		t.Fatalf("pool mismatch: %+v", pool)
	}
}

func TestStatusClusterDecodesWireShape(t *testing.T) {
	client, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v2/status/cluster" {
			t.Fatalf("path = %q", r.URL.Path)
		}
		w.Write([]byte(`{"peers":[{"id":"192.0.2.10"}],"connected":1,"status":"Master","interconnects":{"ethernet":1,"hbactive":0,"hbdrive":0,"serial":0}}`))
	})
	cluster, err := client.Status().Cluster(context.Background())
	if err != nil {
		t.Fatalf("Cluster: %v", err)
	}
	if cluster.Connected != 1 || cluster.Status != "Master" || len(cluster.Peers) != 1 {
		t.Fatalf("cluster mismatch: %+v", cluster)
	}
}

func TestStatusCapacityDecodesPerPoolRecords(t *testing.T) {
	client, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v2/status/capacity" {
			t.Fatalf("path = %q", r.URL.Path)
		}
		w.Write([]byte(`[{"name":"pool1","type":"pool","logical":{"available":"100","reserved":"10","used":"20"},"physical":{"allocated":"200","available":"50","total":"300"},"compress_ratio":"1.0","dedup_ratio":"1.1"}]`))
	})
	capacity, err := client.Status().Capacity(context.Background())
	if err != nil {
		t.Fatalf("Capacity: %v", err)
	}
	if len(capacity) != 1 || capacity[0]["name"] != "pool1" {
		t.Fatalf("capacity mismatch: %+v", capacity)
	}
}

func TestStatusServicesDecodesRunningStoppedMap(t *testing.T) {
	client, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v2/status/services" {
			t.Fatalf("path = %q", r.URL.Path)
		}
		w.Write([]byte(`{"san":"running","nfs":"stopped","cifs":"running"}`))
	})
	services, err := client.Status().Services(context.Background())
	if err != nil {
		t.Fatalf("Services: %v", err)
	}
	if services["san"] != "running" || services["nfs"] != "stopped" {
		t.Fatalf("services mismatch: %+v", services)
	}
}

func TestStatusPerformanceDocumentsDecode(t *testing.T) {
	client, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v2/status/iops":
			w.Write([]byte(`{"read":10,"write":20,"total":30}`))
		case "/api/v2/status/bandwidth":
			w.Write([]byte(`{"read":100,"write":200,"total":300}`))
		case "/api/v2/status/cache":
			w.Write([]byte(`{"dram":{"Current_Size":"10","Hit":"1","Miss":"2"},"flash":{"Cache_Size":"5","Hit_Ratio":"3","Miss_Ratio":"1"}}`))
		case "/api/v2/status/cpu":
			w.Write([]byte(`{"load_1_min":"0.5","load_5_min":"0.4","load_15_min":"0.3","current_idle":"90","average_idle":"85","current_utilization":10,"average_utilization":15}`))
		default:
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
	})
	ctx := context.Background()
	for name, call := range map[string]func() (map[string]interface{}, error){
		"iops":      func() (map[string]interface{}, error) { return client.Status().IOPS(ctx) },
		"bandwidth": func() (map[string]interface{}, error) { return client.Status().Bandwidth(ctx) },
		"cache":     func() (map[string]interface{}, error) { return client.Status().Cache(ctx) },
		"cpu":       func() (map[string]interface{}, error) { return client.Status().CPU(ctx) },
	} {
		doc, err := call()
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if len(doc) == 0 {
			t.Fatalf("%s decoded empty", name)
		}
	}
}

func TestStatusEnclosuresDecodesArray(t *testing.T) {
	client, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v2/status/enclosures" {
			t.Fatalf("path = %q", r.URL.Path)
		}
		w.Write([]byte(`[{"model":"ES1000","no":1,"owner":"ctrl-a","serial":"SN1"}]`))
	})
	enclosures, err := client.Status().Enclosures(context.Background())
	if err != nil {
		t.Fatalf("Enclosures: %v", err)
	}
	if len(enclosures) != 1 || enclosures[0]["serial"] != "SN1" {
		t.Fatalf("enclosures mismatch: %+v", enclosures)
	}
}
