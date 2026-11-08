package database

import (
	"context"
	"database/sql"
	"fmt"

	_ "github.com/jackc/pgx/v5/stdlib"
	_ "modernc.org/sqlite"

	"thinker/internal/config"
)

type DBConn interface {
	Connect(ctx context.Context) (*sql.DB, error)
}

type dbConn struct {
	baseConfig *config.BaseConfig
}

func NewConnector(bc *config.BaseConfig) DBConn {
	return &dbConn{
		baseConfig: bc,
	}
}

// Connect allows define a connection depending on the environment you are working on.
// For debug mode: sqlite (local file)
// For production: postgres through docker container
func (c *dbConn) Connect(ctx context.Context) (*sql.DB, error) {
	var (
		db  *sql.DB
		err error
	)

	if c.baseConfig.Debug {
		db, err = sql.Open("sqlite", "file:internal/database/thinker_dev.db?_pragma=foreign_keys(1)&_pragma=busy_timeout(5000)")
	} else {
		db, err = sql.Open("pgx", c.baseConfig.ConnString)
	}

	if err != nil {
		return nil, fmt.Errorf("Error while opening database: %w", err)
	}

	if err := db.PingContext(ctx); err != nil {
		return nil, fmt.Errorf("Error while connecting to database: %w", err)
	}

	return db, nil

}
