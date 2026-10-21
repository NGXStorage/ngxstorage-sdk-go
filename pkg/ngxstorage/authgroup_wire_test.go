package ngxstorage

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
)

// Wire-contract tests for auth group operations.

const wireAuthGroupDetail = `{
  "id": "4",
  "name": "group1",
  "owner": "ctrl-a",
  "chap_status": 1,
  "chap_username": "chapuser",
  "initiator": [
    {"id": "11", "iqn": "iqn.1994-05.com.redhat:node1", "alias": "host-a"}
  ]
}`

func TestAuthGroupListDecodesLightweightShape(t *testing.T) {
	client, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v2/auth_group" {
			t.Fatalf("path = %q", r.URL.Path)
		}
		w.Write([]byte(`[{"id":"4","name":"group1","owner":"ctrl-a"}]`))
	})
	groups, err := client.AuthGroups().List(context.Background())
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(groups) != 1 || groups[0].ID != "4" {
		t.Fatalf("lightweight list mismatch: %+v", groups)
	}
}

func TestAuthGroupGetDecodesDetailShape(t *testing.T) {
	client, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v2/auth_group/4" {
			t.Fatalf("path = %q", r.URL.Path)
		}
		w.Write([]byte(wireAuthGroupDetail))
	})
	group, err := client.AuthGroups().Get(context.Background(), "4")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if group.Name != "group1" || !group.ChapStatus {
		t.Fatalf("detail decoding mismatch: %+v", group)
	}
	if len(group.Initiators) != 1 || group.Initiators[0].Alias != "host-a" || group.Initiators[0].IQN != "iqn.1994-05.com.redhat:node1" {
		t.Fatalf("initiator decoding mismatch: %+v", group.Initiators)
	}
}

func TestAuthGroupAddCHAPSendsUsernameAndPassword(t *testing.T) {
	var body map[string]interface{}
	client, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPatch || r.URL.Path != "/api/v2/auth_group/chap/4" {
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.Path)
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode body: %v", err)
		}
		w.Write([]byte(`{"success":true}`))
	})
	if err := client.AuthGroups().AddCHAP(context.Background(), "4", "chapuser", "password123456"); err != nil {
		t.Fatalf("AddCHAP: %v", err)
	}
	if body["username"] != "chapuser" || body["password"] != "password123456" {
		t.Fatalf("add-chap body mismatch: %+v", body)
	}
}

func TestAuthGroupDeleteCHAPHitsDeleteRoute(t *testing.T) {
	client, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete || r.URL.Path != "/api/v2/auth_group/chap/4" {
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.Path)
		}
		w.Write([]byte(`{"success":true}`))
	})
	if err := client.AuthGroups().DeleteCHAP(context.Background(), "4"); err != nil {
		t.Fatalf("DeleteCHAP: %v", err)
	}
}

func TestAuthGroupAddIQNSendsOwnerIQNAlias(t *testing.T) {
	var body map[string]interface{}
	client, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPatch || r.URL.Path != "/api/v2/auth_group/iqn/4" {
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.Path)
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode body: %v", err)
		}
		w.Write([]byte(`{"success":true,"id":12}`))
	})
	record, err := client.AuthGroups().AddIQN(context.Background(), "4", "iqn.1994-05.com.redhat:node1", "host-a", "ctrl-a")
	if err != nil {
		t.Fatalf("AddIQN: %v", err)
	}
	if body["owner"] != "ctrl-a" || body["iqn"] != "iqn.1994-05.com.redhat:node1" || body["alias"] != "host-a" {
		t.Fatalf("add-iqn body mismatch: %+v", body)
	}
	if id, _ := record["id"].(float64); id != 12 {
		t.Fatalf("add-iqn record id = %v", record["id"])
	}
}

func TestAuthGroupAddIQNTreatsDuplicateAsSuccess(t *testing.T) {
	client, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"code":522,"error":"IQN already registered"}`))
	})
	record, err := client.AuthGroups().AddIQN(context.Background(), "4", "iqn.1994-05.com.redhat:node1", "host-a", "ctrl-a")
	if err != nil {
		t.Fatalf("duplicate IQN must be treated as already registered: %v", err)
	}
	if len(record) != 0 {
		t.Fatalf("duplicate IQN must return an empty record: %+v", record)
	}
}

func TestAuthGroupDeleteIQNIsIdempotent(t *testing.T) {
	client, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete || r.URL.Path != "/api/v2/auth_group/iqn/12" {
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.Path)
		}
		w.Write([]byte(`{"code":5011,"error":"no iqns"}`))
	})
	if err := client.AuthGroups().DeleteIQN(context.Background(), "12"); err != nil {
		t.Fatalf("DeleteIQN should be idempotent: %v", err)
	}
}
