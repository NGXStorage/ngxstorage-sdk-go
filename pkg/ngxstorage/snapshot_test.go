package ngxstorage

import (
	"encoding/json"
	"testing"
)

// The live backend (verified on V6BGWYPA and SGZ1RVC2, snapshot create and
// get) returns no `ready` field at all. NGX snapshots are synchronous:
// a fetched record is usable immediately.
const liveSnapshotShape = `{"binded":0,"created":"1788950470","id":"08282136ef7e8708ee8c29c3ee88cd820da2037e","name":"probe-snap-tmp","path":"probe-snap-vol@probe-snap-tmp","pool_name":"openstackSSD1","reserved":"57344","restore":1,"vol_size":"1073741824","volume_id":"7176302add549ceaf03caef021707ca0e6021f89","volume_name":"probe-snap-vol","volume_type":"lun"}`

func TestSnapshotUnmarshalLiveShapeDefaultsReady(t *testing.T) {
	var snap Snapshot
	if err := json.Unmarshal([]byte(liveSnapshotShape), &snap); err != nil {
		t.Fatalf("unmarshal snapshot: %v", err)
	}
	if snap.ID != "08282136ef7e8708ee8c29c3ee88cd820da2037e" || snap.Name != "probe-snap-tmp" {
		t.Fatalf("unexpected snapshot identity: %+v", snap)
	}
	if snap.VolumeID != "7176302add549ceaf03caef021707ca0e6021f89" || snap.Size != "57344" {
		t.Fatalf("unexpected snapshot fields: %+v", snap)
	}
	if !snap.Ready {
		t.Fatal("live backend shape without ready field must decode Ready=true")
	}
}

func TestSnapshotUnmarshalHonorsExplicitReady(t *testing.T) {
	for _, tc := range []struct {
		name  string
		wire  string
		ready bool
	}{
		{"absent defaults true", `{"id":"s1","name":"n"}`, true},
		{"null defaults true", `{"id":"s1","name":"n","ready":null}`, true},
		{"bool true", `{"id":"s1","name":"n","ready":true}`, true},
		{"bool false", `{"id":"s1","name":"n","ready":false}`, false},
		{"numeric one", `{"id":"s1","name":"n","ready":1}`, true},
		{"numeric zero", `{"id":"s1","name":"n","ready":0}`, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var snap Snapshot
			if err := json.Unmarshal([]byte(tc.wire), &snap); err != nil {
				t.Fatalf("unmarshal snapshot: %v", err)
			}
			if snap.Ready != tc.ready {
				t.Fatalf("wire %s: Ready=%v, want %v", tc.wire, snap.Ready, tc.ready)
			}
		})
	}
}

func TestSnapshotUnmarshalRejectsInvalidReady(t *testing.T) {
	var snap Snapshot
	if err := json.Unmarshal([]byte(`{"id":"s1","name":"n","ready":"eventually"}`), &snap); err == nil {
		t.Fatal("non-boolean, non-numeric ready should fail decoding")
	}
}
