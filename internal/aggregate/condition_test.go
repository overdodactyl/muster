package aggregate

import "testing"

func TestParseCond(t *testing.T) {
	cases := []struct {
		in   string
		ok   bool
		want Cond
	}{
		{"gpu.gpu_free >= 1", true, Cond{Partition: "gpu", Field: "gpu_free", Op: ">=", Value: 1}},
		{"gpu.gpu_free>=1", true, Cond{Partition: "gpu", Field: "gpu_free", Op: ">=", Value: 1}},
		{"cpu.cpu_free > 100", true, Cond{Partition: "cpu", Field: "cpu_free", Op: ">", Value: 100}},
		{"gpu.idle_nodes == 0", true, Cond{Partition: "gpu", Field: "idle_nodes", Op: "==", Value: 0}},
		{"missing_dot >= 1", false, Cond{}},
		{"gpu.unknown_field >= 1", false, Cond{}},
		{"gpu.gpu_free ?? 1", false, Cond{}},
		{"gpu.gpu_free >= notanumber", false, Cond{}},
	}
	for _, tc := range cases {
		got, err := ParseCond(tc.in)
		if (err == nil) != tc.ok {
			t.Errorf("ParseCond(%q) err=%v, want ok=%v", tc.in, err, tc.ok)
			continue
		}
		if !tc.ok {
			continue
		}
		if *got != tc.want {
			t.Errorf("ParseCond(%q) = %+v, want %+v", tc.in, *got, tc.want)
		}
	}
}

func TestCondEval(t *testing.T) {
	parts := []PartitionSummary{
		{Name: "gpu", TotalCPUs: 100, AllocCPUs: 60, TotalGPUs: 4, AllocGPUs: 3},
	}
	cases := []struct {
		expr    string
		want    bool
		wantVal float64
	}{
		{"gpu.cpu_free >= 40", true, 40},
		{"gpu.cpu_free >= 41", false, 40},
		{"gpu.gpu_free == 1", true, 1},
		{"gpu.gpu_free > 1", false, 1},
		{"gpu.gpu_alloc < 5", true, 3},
	}
	for _, tc := range cases {
		c, err := ParseCond(tc.expr)
		if err != nil {
			t.Fatal(err)
		}
		cur, ok, err := c.Eval(parts)
		if err != nil {
			t.Fatal(err)
		}
		if ok != tc.want {
			t.Errorf("Eval(%q) ok=%v, want %v", tc.expr, ok, tc.want)
		}
		if cur != tc.wantVal {
			t.Errorf("Eval(%q) value=%v, want %v", tc.expr, cur, tc.wantVal)
		}
	}
}
