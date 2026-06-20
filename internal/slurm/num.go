package slurm

import (
	"bytes"
	"encoding/json"
	"time"
)

// slurmNum accepts both the {"set":true,"infinite":false,"number":N} envelope
// (sinfo/squeue/scontrol style) and a bare integer (sacct style).
type slurmNum struct {
	Set      bool  `json:"set"`
	Infinite bool  `json:"infinite"`
	Number   int64 `json:"number"`
}

func (n *slurmNum) UnmarshalJSON(b []byte) error {
	b = bytes.TrimSpace(b)
	if len(b) == 0 || bytes.Equal(b, []byte("null")) {
		return nil
	}
	if b[0] == '{' {
		type raw slurmNum
		var v raw
		if err := json.Unmarshal(b, &v); err != nil {
			return err
		}
		*n = slurmNum(v)
		return nil
	}
	var v int64
	if err := json.Unmarshal(b, &v); err != nil {
		return err
	}
	n.Set = true
	n.Infinite = false
	n.Number = v
	return nil
}

func (n slurmNum) Int() int {
	if !n.Set || n.Infinite {
		return 0
	}
	return int(n.Number)
}

func (n slurmNum) Time() time.Time {
	if !n.Set || n.Infinite || n.Number == 0 {
		return time.Time{}
	}
	return time.Unix(n.Number, 0)
}
