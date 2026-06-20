package cmd

import "muster/internal/slurm"

func newClient() (slurm.Client, error) {
	return slurm.NewCLIClient(), nil
}
