package api

import "testing"

func TestCompareSemver(t *testing.T) {
	tests := []struct {
		a, b string
		want int
	}{
		{a: "v1.2.0", b: "v1.1.9", want: 1},
		{a: "1.2.0", b: "v1.2.0", want: 0},
		{a: "v1.0.0", b: "v1.2.0", want: -1},
		{a: "v2", b: "v1.9.9", want: 1},
	}

	for _, tt := range tests {
		got := compareSemver(tt.a, tt.b)
		if got != tt.want {
			t.Fatalf("compareSemver(%q, %q) = %d, want %d", tt.a, tt.b, got, tt.want)
		}
	}
}
