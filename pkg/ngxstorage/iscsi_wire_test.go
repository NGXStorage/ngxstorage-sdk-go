package ngxstorage

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
)

// Wire-contract tests for iSCSI target operations.

const wireISCSTarget = `{
  "id": "tgt-1",
  "name": "csi-target",
  "owner": "ctrl-a",
  "target_iqn": "iqn.2026-01.com.ngx:csi-target",
  "auth_group_id": "2",
  "auth_group_name": "group1",
  "portal_group_id": "7",
  "portal_group_name": "DefaultPortalGroup",
  "initiators": [
    {"id": "11", "iqn": "iqn.1994-05.com.redhat:node1", "alias": "host-a", "ip": "192.0.2.10"}
  ],
  "luns": [
    {"id": "lun-1", "name": "vol1", "number": "6", "pool_name": "pool1", "scsi_id": "3600"}
  ]
}`

func TestISCSITargetListNamesDecodesLightweightShape(t *testing.T) {
	client, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v2/target/iscsi" {
			t.Fatalf("path = %q", r.URL.Path)
		}
		w.Write([]byte(`[{"id":"tgt-1","name":"csi-target","owner":"ctrl-a"}]`))
	})
	targets, err := client.ISCSITargets().ListNames(context.Background())
	if err != nil {
		t.Fatalf("ListNames: %v", err)
	}
	if len(targets) != 1 || targets[0].ID != "tgt-1" || targets[0].Owner != "ctrl-a" {
		t.Fatalf("lightweight list mismatch: %+v", targets)
	}
}

func TestISCSITargetGetDecodesFullShape(t *testing.T) {
	client, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v2/target/iscsi/tgt-1" {
			t.Fatalf("path = %q", r.URL.Path)
		}
		w.Write([]byte(wireISCSTarget))
	})
	target, err := client.ISCSITargets().Get(context.Background(), "tgt-1")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if target.TargetIQN != "iqn.2026-01.com.ngx:csi-target" ||
		target.AuthGroupID != "2" || target.PortalGroupName != "DefaultPortalGroup" {
		t.Fatalf("target identity mismatch: %+v", target)
	}
	if len(target.Initiators) != 1 || target.Initiators[0].IP != "192.0.2.10" || target.Initiators[0].Alias != "host-a" {
		t.Fatalf("initiator decoding mismatch: %+v", target.Initiators)
	}
	if len(target.Luns) != 1 || target.Luns[0].ScsiID != "3600" || target.Luns[0].PoolName != "pool1" {
		t.Fatalf("lun decoding mismatch: %+v", target.Luns)
	}
}

func TestISCSITargetGetDecodesNumericLUNNumber(t *testing.T) {
	client, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"id":"tgt-1","name":"t","owner":"o","luns":[{"id":"l1","name":"v","number":6,"pool_name":"p","scsi_id":"s"}]}`))
	})
	target, err := client.ISCSITargets().Get(context.Background(), "tgt-1")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if target.Luns[0].Number != "6" {
		t.Fatalf("numeric LUN number = %q, want \"6\"", target.Luns[0].Number)
	}
}

func TestISCSITargetChangeNameSendsOwnerAndName(t *testing.T) {
	var body map[string]interface{}
	client, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPatch || r.URL.Path != "/api/v2/target/iscsi/tgt-1" {
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.Path)
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode body: %v", err)
		}
		w.Write([]byte(`{"success":true}`))
	})
	if err := client.ISCSITargets().ChangeName(context.Background(), "tgt-1", "new-name", "ctrl-a"); err != nil {
		t.Fatalf("ChangeName: %v", err)
	}
	if body["name"] != "new-name" || body["owner"] != "ctrl-a" {
		t.Fatalf("change-name body mismatch: %+v", body)
	}
}

func TestISCSITargetChangeAuthGroupHitsDedicatedRoute(t *testing.T) {
	var body map[string]interface{}
	client, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPatch || r.URL.Path != "/api/v2/target/iscsi/auth_group/tgt-1" {
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.Path)
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode body: %v", err)
		}
		w.Write([]byte(`{"success":true}`))
	})
	if err := client.ISCSITargets().ChangeAuthGroup(context.Background(), "tgt-1", "3", "ctrl-a"); err != nil {
		t.Fatalf("ChangeAuthGroup: %v", err)
	}
	if body["auth_group_id"] != "3" || body["owner"] != "ctrl-a" {
		t.Fatalf("change-auth-group body mismatch: %+v", body)
	}
}

func TestISCSITargetChangePortalGroupHitsDedicatedRoute(t *testing.T) {
	var body map[string]interface{}
	client, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPatch || r.URL.Path != "/api/v2/target/iscsi/portal_group/tgt-1" {
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.Path)
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode body: %v", err)
		}
		w.Write([]byte(`{"success":true}`))
	})
	if err := client.ISCSITargets().ChangePortalGroup(context.Background(), "tgt-1", "40hexportalgroupid40hexportalgroupid40hexp", "ctrl-a"); err != nil {
		t.Fatalf("ChangePortalGroup: %v", err)
	}
	if _, ok := body["portal_group_id"]; !ok || body["owner"] != "ctrl-a" {
		t.Fatalf("change-portal-group body mismatch: %+v", body)
	}
}

func TestISCSITargetAddRemoveLUNSendLunIDs(t *testing.T) {
	var bodies []map[string]interface{}
	client, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		body := map[string]interface{}{}
		if r.Body != nil {
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Fatalf("decode body: %v", err)
			}
		}
		bodies = append(bodies, body)
		w.Write([]byte(`{"success":true}`))
	})
	if err := client.ISCSITargets().AddLUN(context.Background(), "tgt-1", "lun-1", "ctrl-a"); err != nil {
		t.Fatalf("AddLUN: %v", err)
	}
	if err := client.ISCSITargets().RemoveLUN(context.Background(), "tgt-1", "lun-1", "ctrl-a"); err != nil {
		t.Fatalf("RemoveLUN: %v", err)
	}
	for i, body := range bodies {
		luns, ok := body["lun_ids"].([]interface{})
		if !ok || len(luns) != 1 || luns[0] != "lun-1" || body["owner"] != "ctrl-a" {
			t.Fatalf("request %d lun_ids/owner mismatch: %+v", i, body)
		}
	}
}

func TestISCSITargetDeleteIdempotentOnSemanticNotFound(t *testing.T) {
	client, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"code":5011,"error":"no target"}`))
	})
	if err := client.ISCSITargets().Delete(context.Background(), "gone-1"); err != nil {
		t.Fatalf("Delete should be idempotent: %v", err)
	}
}
