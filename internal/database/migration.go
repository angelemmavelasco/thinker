package database

import (
	"context"
	"database/sql"
	_ "embed"
	"fmt"
)

//go:embed schema.sql
var schemaSQL string

func InitSchema(c context.Context, conn *sql.DB) error {
	_, err := conn.ExecContext(c, schemaSQL)
	if err != nil {
		return fmt.Errorf("Error while defining base schema at database: %w", err)
	}

	return nil
}
