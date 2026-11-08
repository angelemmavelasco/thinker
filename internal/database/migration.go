package database

import (
	"context"
	"database/sql"
	_ "embed"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

//go:embed schema.sql
var schemaSQL string

const migrationsDir = "internal/database/migrations"
const developmentUserID = "00000000-0000-0000-0000-000000000001"

const createMigrationsTableSQL = `
CREATE TABLE IF NOT EXISTS schema_migrations (
    version VARCHAR(255) PRIMARY KEY,
    applied_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);
`

func InitSchema(ctx context.Context, conn *sql.DB) error {

	if _, err := conn.ExecContext(ctx, createMigrationsTableSQL); err != nil {
		return fmt.Errorf("error while creating schema_migrations table: %w", err)
	}

	if _, err := conn.ExecContext(ctx, schemaSQL); err != nil {
		return fmt.Errorf("error while defining base schema at database: %w", err)
	}

	if err := syncSchemaColumns(ctx, conn, schemaSQL); err != nil {
		return fmt.Errorf("error while syncing schema columns: %w", err)
	}

	if err := runVersionedMigrations(ctx, conn); err != nil {
		return fmt.Errorf("error while running versioned migrations: %w", err)
	}

	return nil
}

func EnsureDevelopmentUser(ctx context.Context, conn *sql.DB) error {
	const query = `
		INSERT INTO users (id, email, password_hash)
		VALUES ($1, $2, $3)
		ON CONFLICT (id) DO NOTHING
	`

	_, err := conn.ExecContext(
		ctx,
		query,
		developmentUserID,
		"development@thinker.local",
		"development-only",
	)
	return err
}

func runVersionedMigrations(ctx context.Context, conn *sql.DB) error {
	entries, err := os.ReadDir(migrationsDir)
	if err != nil {
		return nil
	}

	var files []string
	for _, entry := range entries {
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".sql") {
			files = append(files, entry.Name())
		}
	}
	sort.Strings(files)

	for _, fileName := range files {
		var count int
		err := conn.QueryRowContext(
			ctx,
			"SELECT COUNT(1) FROM schema_migrations WHERE version = $1",
			fileName,
		).Scan(&count)
		if err != nil {
			return fmt.Errorf("failed checking migration %s: %w", fileName, err)
		}

		if count > 0 {
			continue
		}

		content, err := os.ReadFile(filepath.Join(migrationsDir, fileName))
		if err != nil {
			return fmt.Errorf("failed reading migration file %s: %w", fileName, err)
		}

		tx, err := conn.BeginTx(ctx, nil)
		if err != nil {
			return fmt.Errorf("failed starting transaction for %s: %w", fileName, err)
		}

		if strings.TrimSpace(string(content)) != "" {
			if _, err := tx.ExecContext(ctx, string(content)); err != nil {
				_ = tx.Rollback()
				return fmt.Errorf("migration %s failed: %w", fileName, err)
			}
		}

		if _, err := tx.ExecContext(
			ctx,
			"INSERT INTO schema_migrations (version) VALUES ($1)",
			fileName,
		); err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("failed recording migration %s: %w", fileName, err)
		}

		if err := tx.Commit(); err != nil {
			return fmt.Errorf("failed committing migration %s: %w", fileName, err)
		}

		log.Printf("[migrations] Applied migration: %s", fileName)
	}

	return nil
}

var createTableRegex = regexp.MustCompile(`(?is)CREATE\s+TABLE\s+(?:IF\s+NOT\s+EXISTS\s+)?([a-zA-Z0-9_]+)\s*\((.*?)\)\s*;`)

func syncSchemaColumns(ctx context.Context, conn *sql.DB, rawSchema string) error {
	matches := createTableRegex.FindAllStringSubmatch(rawSchema, -1)
	for _, match := range matches {
		tableName := strings.TrimSpace(match[1])
		body := match[2]

		existingCols, err := getExistingColumns(ctx, conn, tableName)
		if err != nil {
			return err
		}

		lines := strings.Split(body, "\n")
		for _, line := range lines {
			colDef := strings.TrimSpace(line)
			if colDef == "" || strings.HasPrefix(colDef, "--") {
				continue
			}
			colDef = strings.TrimRight(colDef, ",")
			colDef = strings.TrimSpace(colDef)
			if colDef == "" {
				continue
			}

			parts := strings.Fields(colDef)
			if len(parts) < 2 {
				continue
			}

			firstWordUpper := strings.ToUpper(parts[0])

			if firstWordUpper == "PRIMARY" ||
				firstWordUpper == "FOREIGN" ||
				firstWordUpper == "UNIQUE" ||
				firstWordUpper == "CHECK" ||
				firstWordUpper == "CONSTRAINT" {
				continue
			}

			colName := strings.Trim(parts[0], `"`+"`")
			if !existingCols[strings.ToLower(colName)] {
				alterSQL := fmt.Sprintf("ALTER TABLE %s ADD COLUMN %s", tableName, colDef)
				if _, err := conn.ExecContext(ctx, alterSQL); err != nil {
					return fmt.Errorf("failed adding column %s.%s: %w", tableName, colName, err)
				}
				log.Printf("[migrations] Auto-added new column from schema.sql: %s.%s", tableName, colName)
			}
		}
	}
	return nil
}

func getExistingColumns(ctx context.Context, conn *sql.DB, tableName string) (map[string]bool, error) {
	rows, err := conn.QueryContext(ctx, fmt.Sprintf("SELECT * FROM %s LIMIT 0", tableName))
	if err != nil {
		return nil, err
	}
	defer func() {
		if closeErr := rows.Close(); err == nil && closeErr != nil {
			err = fmt.Errorf("error closing database rows: %w", closeErr)
		}
	}()

	cols, err := rows.Columns()
	if err != nil {
		return nil, err
	}

	result := make(map[string]bool, len(cols))
	for _, c := range cols {
		result[strings.ToLower(c)] = true
	}
	return result, nil
}
