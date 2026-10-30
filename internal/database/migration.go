package database

import (
	"context"
	_ "embed"
	"fmt"

	"github.com/jackc/pgx/v5"
)

//go:embed schema.sql
var schemaSQL string

func InitSchema(c context.Context, conn *pgx.Conn) error {
	_, err := conn.Exec(c, schemaSQL)
	if err != nil {
		return fmt.Errorf("Error while defining base schema at database: %w", err)
	}

	return nil
}
