package databases

import (
	"context"

	"github.com/kodek/tesler/recorder/car"
)

type Database interface {
	GetLatest(ctx context.Context) (*car.Snapshot, error)

	Insert(ctx context.Context, info car.Snapshot) error

	Close() error
}
