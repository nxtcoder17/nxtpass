package store

import (
	"context"
	"io"

	"github.com/nxtcoder17/nxtpass/server/internal/store/models"
	"github.com/nxtcoder17/nxtpass/server/internal/ulid"
)

type Store interface {
	Create(ctx context.Context, cred models.Credential) (ulid.ID, error)
	List(ctx context.Context, namespace string)
	Delete(ctx context.Context, id ulid.ID)
	LastCheckpointAt(ctx context.Context) (int64, error)
	ChangeStream(ctx context.Context, since int64, writer io.Writer) error
	SyncRecord(ctx context.Context, timestamp int64, query string) error
}
