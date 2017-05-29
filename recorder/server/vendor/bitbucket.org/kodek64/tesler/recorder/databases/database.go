package databases

import (
	"context"

	"bitbucket.org/kodek64/tesler/recorder/car"
)

type Database interface {
	GetLatest(ctx context.Context) (*car.Snapshot, error)

	Insert(ctx context.Context, info car.Snapshot) error

	Close() error
}
