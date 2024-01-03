package database

import (
	"context"
	"errors"
	"os"
)

func IsConnected(ctx context.Context, db BoltDriver) error {
	_, err := os.Stat(db.Path())
	if err != nil {
		return err
	}

	if db.IsReadOnly() {
		return errors.New("invalid database storage file: read-only")
	}

	return nil
}
