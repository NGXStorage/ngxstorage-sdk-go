package ngxstorage

import (
	"encoding/json"
	"testing"
)

func TestPoolUnmarshalAcceptsCanonicalFields(t *testing.T) {
	var pool Pool
	err := json.Unmarshal([]byte(`{"id":"p1","name":"test1","owner":"ctrl-b","blocksize":"16k"}`), &pool)
	if err != nil {
		t.Fatalf("unmarshal pool: %v", err)
	}
	if pool.ID != "p1" || pool.Name != "test1" || pool.Owner != "ctrl-b" || pool.BlockSize != "16k" {
		t.Fatalf("unexpected pool: %+v", pool)
	}
}

func TestPoolUnmarshalKeepsCanonicalFields(t *testing.T) {
	var pool Pool
	err := json.Unmarshal([]byte(`{"id":"p2","name":"canonical","owner":"ctrl-a"}`), &pool)
	if err != nil {
		t.Fatalf("unmarshal pool: %v", err)
	}
	if pool.ID != "p2" || pool.Name != "canonical" || pool.Owner != "ctrl-a" {
		t.Fatalf("unexpected pool: %+v", pool)
	}
}

func TestPoolUnmarshalAcceptsNumericIsFlash(t *testing.T) {
	var pool Pool
	if err := json.Unmarshal([]byte(`{"id":"p1","name":"test1","is_flash":1}`), &pool); err != nil {
		t.Fatalf("unmarshal pool: %v", err)
	}
	if !pool.IsFlash {
		t.Fatal("is_flash=1 should decode true")
	}
}
