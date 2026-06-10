package db

import (
	"context"
	"database/sql"
	"errors"
)

func Connect(ctx context.Context, databaseURL string) (*sql.DB, error) {
	if databaseURL == "" {
		return nil, errors.New("database URL is required")
	}

	return nil, errors.New("database connection is not implemented in phase 1")
}
