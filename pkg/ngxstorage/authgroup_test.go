package ngxstorage

import (
	"encoding/json"
	"testing"
)

// TestAuthGroupUnmarshalChapStatus covers the live failure that motivated the
// FlexBool field: the backend returns chap_status as an integer (0/1) while
// older software returned a JSON boolean.
func TestAuthGroupUnmarshalChapStatus(t *testing.T) {
	cases := []struct {
		name string
		raw  string
		want bool
	}{
		{name: "integer enabled", raw: `{"id":"ag-1","name":"ag","chap_status":1}`, want: true},
		{name: "integer disabled", raw: `{"id":"ag-1","name":"ag","chap_status":0}`, want: false},
		{name: "boolean enabled", raw: `{"id":"ag-1","name":"ag","chap_status":true}`, want: true},
		{name: "boolean disabled", raw: `{"id":"ag-1","name":"ag","chap_status":false}`, want: false},
		{name: "string enabled", raw: `{"id":"ag-1","name":"ag","chap_status":"1"}`, want: true},
		{name: "missing", raw: `{"id":"ag-1","name":"ag"}`, want: false},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			var group AuthGroup
			if err := json.Unmarshal([]byte(test.raw), &group); err != nil {
				t.Fatalf("Unmarshal(%s): %v", test.raw, err)
			}
			if group.ID != "ag-1" {
				t.Fatalf("ID = %q, want ag-1", group.ID)
			}
			if bool(group.ChapStatus) != test.want {
				t.Fatalf("ChapStatus = %v, want %v", bool(group.ChapStatus), test.want)
			}
		})
	}
}
