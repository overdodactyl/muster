package slurm

import (
	"context"
	"time"
)

type Client interface {
	Nodes(ctx context.Context) ([]Node, error)
	Jobs(ctx context.Context, partition string) ([]Job, error)
	Partitions(ctx context.Context) ([]Partition, error)
	Accounting(ctx context.Context, since time.Duration, partition string) ([]AcctJob, error)
	Reservations(ctx context.Context) ([]Reservation, error)
	SlurmVersion(ctx context.Context) (string, error)
}
