package update

import "testing"

func TestVersionGreaterThan(t *testing.T) {
	tests := []struct {
		v, w string
		want bool
	}{
		{"v0.5.1", "v0.5.0", true},
		{"0.5.1", "0.5.0", true},
		{"v0.5.0", "v0.5.0", false},
		{"v0.4.0", "v0.5.0", false},
		{"v1.0.0", "v0.9.9", true},
		{"v0.10.0", "v0.9.0", true},
		{"v2.0.0", "v1.99.99", true},
		{"v0.5.1", "dev", false},
		{"dev", "v0.5.0", false},
		{"invalid", "v0.5.0", false},
		{"v0.5.0", "invalid", false},
	}

	for _, tt := range tests {
		t.Run(tt.v+"_vs_"+tt.w, func(t *testing.T) {
			got := versionGreaterThan(tt.v, tt.w)
			if got != tt.want {
				t.Errorf("versionGreaterThan(%q, %q) = %v, want %v", tt.v, tt.w, got, tt.want)
			}
		})
	}
}
