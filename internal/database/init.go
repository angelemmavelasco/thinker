package database

import (
	"context"
	"fmt"
	"thinker/internal/config"

	"github.com/jackc/pgx/v5"
)

type DBConn interface {
	Connect(ctx context.Context) (*pgx.Conn, error)
}

type dbConn struct {
	baseConfig *config.BaseConfig
}

func NewConnector(bc *config.BaseConfig) DBConn {
	return &dbConn{
		baseConfig: bc,
	}
}

func (c *dbConn) Connect(ctx context.Context) (*pgx.Conn, error) {
	conn, err := pgx.Connect(ctx, c.baseConfig.ConnString)
	if err != nil {
		return nil, fmt.Errorf("error while initializing database: %w", err)
	}

	return conn, nil
}
