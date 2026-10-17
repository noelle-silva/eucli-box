package types

import "testing"

func TestCacheRateFromHitMiss(t *testing.T) {
	cases := []struct {
		name string
		hit  int
		miss int
		want *float64
	}{
		{name: "hit and miss", hit: 80, miss: 20, want: floatPtr(0.8)},
		{name: "all miss", hit: 0, miss: 10, want: floatPtr(0)},
		{name: "all hit", hit: 10, miss: 0, want: floatPtr(1)},
		{name: "no input", hit: 0, miss: 0, want: nil},
		{name: "negative hit", hit: -1, miss: 10, want: nil},
		{name: "negative miss", hit: 10, miss: -1, want: nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := CacheRateFromHitMiss(tc.hit, tc.miss)
			if tc.want == nil {
				if got != nil {
					t.Fatalf("CacheRateFromHitMiss(%d, %d) = %v, want nil", tc.hit, tc.miss, *got)
				}
				return
			}
			if got == nil || *got != *tc.want {
				t.Fatalf("CacheRateFromHitMiss(%d, %d) = %v, want %v", tc.hit, tc.miss, got, *tc.want)
			}
		})
	}
}

func floatPtr(value float64) *float64 {
	return &value
}
