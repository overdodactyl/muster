package slurm

import (
	"reflect"
	"testing"
)

func TestParseGRES(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want []GRES
	}{
		{"empty", "", nil},
		{"null", "(null)", nil},
		{"modelless", "gpu:4", []GRES{{Kind: "gpu", Count: 4}}},
		{"with model", "gpu:a100:4", []GRES{{Kind: "gpu", Model: "a100", Count: 4}}},
		{"with idx range", "gpu:a100:3(IDX:0-2)", []GRES{{Kind: "gpu", Model: "a100", Count: 3, Index: []int{0, 1, 2}}}},
		{"idx singles+range", "gpu:a100:3(IDX:0,2-3)", []GRES{{Kind: "gpu", Model: "a100", Count: 3, Index: []int{0, 2, 3}}}},
		{"multi", "gpu:a100:4,nvme:1", []GRES{{Kind: "gpu", Model: "a100", Count: 4}, {Kind: "nvme", Count: 1}}},
		{"malformed silently dropped", "garbage", nil},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := ParseGRES(tc.in)
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("ParseGRES(%q) = %#v, want %#v", tc.in, got, tc.want)
			}
		})
	}
}

func TestGPUCount(t *testing.T) {
	got := GPUCount(ParseGRES("gpu:a100:4,nvme:1"))
	if got != 4 {
		t.Errorf("GPUCount = %d, want 4", got)
	}
}

func TestGPUModel(t *testing.T) {
	if m := GPUModel(ParseGRES("gpu:a100:4")); m != "a100" {
		t.Errorf("GPUModel = %q, want a100", m)
	}
	if m := GPUModel(ParseGRES("gpu:4")); m != "" {
		t.Errorf("GPUModel = %q, want empty", m)
	}
}
