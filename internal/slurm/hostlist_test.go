package slurm

import (
	"reflect"
	"testing"
)

func TestExpandHostlist(t *testing.T) {
	cases := []struct {
		in   string
		want []string
	}{
		{"", nil},
		{"node013a", []string{"node013a"}},
		{"nodeA,nodeB", []string{"nodeA", "nodeB"}},
		{"node[013-014]a", []string{"node013a", "node014a"}},
		{"node[013-014,016]a", []string{"node013a", "node014a", "node016a"}},
		{"prefix[01-02],other[1-2]", []string{"prefix01", "prefix02", "other1", "other2"}},
	}
	for _, c := range cases {
		got, err := ExpandHostlist(c.in)
		if err != nil {
			t.Errorf("ExpandHostlist(%q) error: %v", c.in, err)
			continue
		}
		if !reflect.DeepEqual(got, c.want) {
			t.Errorf("ExpandHostlist(%q) = %v, want %v", c.in, got, c.want)
		}
	}
}
