package slurm

import "testing"

func TestParseArrayTaskString(t *testing.T) {
	cases := []struct {
		in           string
		wantCount    int
		wantThrottle int
	}{
		{"73-224%3", 152, 3},
		{"1,3,5", 3, 0},
		{"1-3,7,10-12", 7, 0},
		{"1-100:2%4", 50, 4}, // 1,3,5,...,99 = 50 values
		{"1-100:2", 50, 0},
		{"5", 1, 0},
		{"", 0, 0},
		{"garbage", 0, 0},
		{"1-", 0, 0},
		{"10-5", 0, 0},
		{"1-3%0", 3, 0},
		{"0-9%2", 10, 2},
	}
	for _, c := range cases {
		gotC, gotT := parseArrayTaskString(c.in)
		if gotC != c.wantCount || gotT != c.wantThrottle {
			t.Errorf("parseArrayTaskString(%q) = (%d, %d); want (%d, %d)",
				c.in, gotC, gotT, c.wantCount, c.wantThrottle)
		}
	}
}
