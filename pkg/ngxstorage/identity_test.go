package ngxstorage

import "testing"

func TestIsCanonicalVolumeID(t *testing.T) {
	tests := []struct {
		id   string
		want bool
	}{
		{"a", true},
		{"0123456789abcdef0123456789abcdef01234567", true},
		{"ABCDEF0123456789", true},
		{"", false},
		{"g", false},
		{"12345678901234567890123456789012345678901", false},
		{" 0123", false},
		{"0123/", false},
	}
	for _, tt := range tests {
		if got := IsCanonicalVolumeID(tt.id); got != tt.want {
			t.Errorf("IsCanonicalVolumeID(%q) = %t, want %t", tt.id, got, tt.want)
		}
	}
}
