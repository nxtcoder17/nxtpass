package store

import (
	"context"

	"github.com/nxtcoder17/nxtpass/server/internal/store/internal/sqlite"
)

func NewSQLiteStore(file string) (Store, error) {
	db, err := sqlite.Connect(context.TODO(), file)
	if err != nil {
		return nil, err
	}
	return &sqlite.Store{DB: db}, nil
}
