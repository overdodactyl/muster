package cmd

import (
	"os"

	"muster/internal/slurm"
)

func newClient() (slurm.Client, error) {
	if dir := fixturesDir(); dir != "" {
		return slurm.NewFixtureClient(dir), nil
	}
	return slurm.NewCLIClient(), nil
}

// fixturesDir returns the fixture-mode directory if either the --fixtures flag
// or the MUSTER_FIXTURES env var is set. Flag wins over env.
func fixturesDir() string {
	if flagFixtures != "" {
		return flagFixtures
	}
	return os.Getenv("MUSTER_FIXTURES")
}
