package ngxstorage

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
)

// These tests pin the wire contracts of operations whose previous SDK shapes
// were rejected by the deployed backend (400/5008 or undecodable responses).

func TestSnapshotScheduleContract(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v2/snapshot/schedule/vol-1":
			if r.Method != http.MethodPost {
				t.Fatalf("method = %s, want POST", r.Method)
			}
			var body map[string]interface{}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Fatalf("decode schedule body: %v", err)
			}
			if body["cron_time"] != "*/10 * * * *" || body["keep_max"] != float64(32) {
				t.Fatalf("schedule body must be cron_time + keep_max: %+v", body)
			}
			w.Write([]byte(`{"success": true, "h_read": "Every 10 minutes"}`))
		default:
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
	})
	client, _ := newTestClient(t, handler)
	hRead, err := client.Snapshots().CreateSchedule(context.Background(), "vol-1", "*/10 * * * *", 32)
	if err != nil {
		t.Fatalf("CreateSchedule: %v", err)
	}
	if hRead != "Every 10 minutes" {
		t.Fatalf("h_read = %q", hRead)
	}
}

func TestSnapshotScheduleValidation(t *testing.T) {
	client, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("out-of-range keep_max must not reach the backend")
	})
	for _, keepMax := range []int{0, 513} {
		if _, err := client.Snapshots().CreateSchedule(context.Background(), "vol-1", "*/10 * * * *", keepMax); err == nil {
			t.Fatalf("keep_max %d should be rejected", keepMax)
		}
	}
}

func TestSnapshotScheduleDetailDecodesWireShape(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"c_time":"*/10 * * * *","h_read":"Every 10 minutes","id":"sch-1","keep_max":"32","owner":"V6BGWYPA_B","volume_id":"vol-1","volume_type":"share"}`))
	})
	client, _ := newTestClient(t, handler)
	schedule, err := client.Snapshots().GetSchedule(context.Background(), "vol-1")
	if err != nil {
		t.Fatalf("GetSchedule: %v", err)
	}
	if schedule.CronTime != "*/10 * * * *" || schedule.KeepMax != "32" || schedule.HumanRead != "Every 10 minutes" || schedule.VolumeType != "share" {
		t.Fatalf("schedule wire decode mismatch: %+v", schedule)
	}
}

func TestFCInitiatorTagContract(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v2/target/fc/initiator/tag" || r.Method != http.MethodPost {
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.Path)
		}
		var body map[string]interface{}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode tag body: %v", err)
		}
		if body["initiator_naa"] != "5006016000000000" || body["tag"] != "tag-a" {
			t.Fatalf("tag body must be initiator_naa + tag: %+v", body)
		}
		for _, rejected := range []string{"name", "initiator_ids", "initiator_wwn"} {
			if _, ok := body[rejected]; ok {
				t.Fatalf("tag body must not send %s: %+v", rejected, body)
			}
		}
		w.Write([]byte(`{"id":"tag-1","initiator_naa":"5006016000000000","tag":"tag-a"}`))
	})
	client, _ := newTestClient(t, handler)
	tag, err := client.FCTargets().CreateInitiatorTag(context.Background(), "5006016000000000", "tag-a")
	if err != nil {
		t.Fatalf("CreateInitiatorTag: %v", err)
	}
	if tag.ID != "tag-1" || tag.InitiatorNAA != "5006016000000000" || tag.Tag != "tag-a" {
		t.Fatalf("tag decode mismatch: %+v", tag)
	}
}

func TestPoolOverviewReturnsArray(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v2/pool/overview" {
			t.Fatalf("path = %q", r.URL.Path)
		}
		w.Write([]byte(`[{"blocksize":"32k","pool_name":"p1","state":"ONLINE"}]`))
	})
	client, _ := newTestClient(t, handler)
	records, err := client.Pools().Overview(context.Background())
	if err != nil {
		t.Fatalf("Overview: %v", err)
	}
	if len(records) != 1 || records[0]["pool_name"] != "p1" {
		t.Fatalf("overview records mismatch: %+v", records)
	}
}

func TestOverloadedVolumesDecodesWrappedDocument(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"last_check_time":1756716344.5,"overloaded_volumes":[{"id":"v1","name":"vol","size":10,"used":9,"type":"lun"}]}`))
	})
	client, _ := newTestClient(t, handler)
	report, err := client.Status().OverloadedVolumes(context.Background())
	if err != nil {
		t.Fatalf("OverloadedVolumes: %v", err)
	}
	if len(report.OverloadedVolumes) != 1 || report.LastCheckTime != 1756716344.5 {
		t.Fatalf("report mismatch: %+v", report)
	}
}

func TestUserCreateDecodesNumericID(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v2/user" || r.Method != http.MethodPost {
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.Path)
		}
		w.Write([]byte(`{"id":1758180347}`))
	})
	client, _ := newTestClient(t, handler)
	id, err := client.Users().Create(context.Background(), map[string]interface{}{
		"username": "sdkuser", "password": "secret123", "role": "monitor",
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if id != "1758180347" {
		t.Fatalf("user id = %q, want 1758180347", id)
	}
}

func TestPortalGroupDetailList(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v2/portal_group/list" {
			t.Fatalf("path = %q", r.URL.Path)
		}
		w.Write([]byte(`[{"id":"7","name":"DefaultPortalGroup","owner":"V6BGWYPA_B","listen_ips":[{"id":"1","interface_name":"em1","ip_address":"192.0.2.10","owner":"V6BGWYPA_B"}]}]`))
	})
	client, _ := newTestClient(t, handler)
	groups, err := client.PortalGroups().ListDetail(context.Background())
	if err != nil {
		t.Fatalf("ListDetail: %v", err)
	}
	if len(groups) != 1 || groups[0].ID != "7" || len(groups[0].IPAddresses()) != 1 {
		t.Fatalf("portal group detail mismatch: %+v", groups)
	}
}

func TestISCSITargetCreateSendsRequiredKeys(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v2/target/iscsi" || r.Method != http.MethodPost {
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.Path)
		}
		var body map[string]interface{}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode create body: %v", err)
		}
		for _, key := range []string{"name", "auth_group_id", "portal_group_id", "owner"} {
			if _, ok := body[key]; !ok {
				t.Fatalf("create body missing required key %s: %+v", key, body)
			}
		}
		// The backend acknowledges with only {"success": true}.
		w.Write([]byte(`{"success":true}`))
	})
	client, _ := newTestClient(t, handler)
	if err := client.ISCSITargets().Create(context.Background(), "tgt-1", "2", "40-hex-portal-id", "V6BGWYPA_B"); err != nil {
		t.Fatalf("Create: %v", err)
	}
}

func TestInitiatorListUsesBareRouteWithoutID(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v2/initiators/fc", "/api/v2/initiators/iscsi":
			w.Write([]byte(`[]`))
		default:
			t.Fatalf("path = %q (an empty id must not leak into the URL)", r.URL.Path)
		}
	})
	client, _ := newTestClient(t, handler)
	if _, err := client.Initiators().GetFC(context.Background(), ""); err != nil {
		t.Fatalf("GetFC: %v", err)
	}
	if _, err := client.Initiators().GetISCSI(context.Background(), ""); err != nil {
		t.Fatalf("GetISCSI: %v", err)
	}
}

func TestISCSITargetDetailDecodesLUNNumber(t *testing.T) {
	// The backend serializes the SCSI LUN number as a JSON integer;
	// older releases emit a quoted string.
	for _, wire := range []string{
		`{"id":"t1","name":"tgt","owner":"o","luns":[{"id":"l1","name":"v","number":6,"pool_name":"p","scsi_id":"s"}]}`,
		`{"id":"t1","name":"tgt","owner":"o","luns":[{"id":"l1","name":"v","number":"6","pool_name":"p","scsi_id":"s"}]}`,
	} {
		handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Write([]byte(wire))
		})
		client, _ := newTestClient(t, handler)
		target, err := client.ISCSITargets().Get(context.Background(), "t1")
		if err != nil {
			t.Fatalf("Get(%s): %v", wire, err)
		}
		if target.Luns[0].Number != "6" {
			t.Fatalf("number = %q, want \"6\" for wire %s", target.Luns[0].Number, wire)
		}
	}
}

func TestAuthGroupCreateDecodesNumericID(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"id":4,"success":true}`))
	})
	client, _ := newTestClient(t, handler)
	id, err := client.AuthGroups().Create(context.Background(), "group1", "V6BGWYPA_B")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if id != "4" {
		t.Fatalf("auth group id = %q, want 4", id)
	}
}
