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
	Cancel(ctx context.Context, jobID int64) error
	JobDetail(ctx context.Context, jobID int64) (JobDetail, error)
	JobEfficiency(ctx context.Context, jobID int64) (JobEfficiency, error)
	JobsByName(ctx context.Context, name, partition string, since time.Duration) ([]AcctJob, error)
	ClusterName(ctx context.Context) (string, error)
	SlurmVersion(ctx context.Context) (string, error)
}
