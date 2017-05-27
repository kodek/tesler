package databases

import (
	"context"

	"bitbucket.org/kodek64/tesler/recorder"
)

type Database interface {
	GetLatest(ctx context.Context) (*recorder.CarInfo, error)

	Insert(ctx context.Context, info *recorder.CarInfo) error

	Close() error
}
