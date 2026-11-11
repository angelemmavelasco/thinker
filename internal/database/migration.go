package database

import (
	"context"
	"database/sql"
	"embed"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/google/uuid"
)

//go:embed schema.sql
var schemaSQL string

//go:embed migrations/*.sql
var migrationsFS embed.FS

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

	if err := backfillIdeaGroups(ctx, conn); err != nil {
		return fmt.Errorf("error while backfilling idea groups: %w", err)
	}

	if err := seedDefaultKnowledgeSettings(ctx, conn); err != nil {
		return fmt.Errorf("error while seeding default knowledge settings: %w", err)
	}

	return nil
}

func seedDefaultKnowledgeSettings(ctx context.Context, conn *sql.DB) error {
	defaults := [][2]string{
		{"allow_auto_moc_creation", "true"},
		{"hub_dedup_similarity_threshold", "0.82"},
		{"max_auto_mocs", "15"},
	}
	for _, pair := range defaults {
		if _, err := conn.ExecContext(ctx,
			"INSERT INTO knowledge_settings (key, value) VALUES ($1, $2) ON CONFLICT (key) DO NOTHING",
			pair[0], pair[1],
		); err != nil {
			return err
		}
	}
	return nil
}

var chunkTitleSuffixRegex = regexp.MustCompile(`^(.*)\s+\((\d+)/(\d+)\)$`)

func backfillIdeaGroups(ctx context.Context, conn *sql.DB) error {
	rows, err := conn.QueryContext(
		ctx,
		"SELECT id, title FROM ideas WHERE group_id IS NULL OR group_id = '00000000-0000-0000-0000-000000000000' OR CAST(group_id AS TEXT) = ''",
	)
	if err != nil {
		return err
	}
	defer rows.Close()

	type pendingRow struct {
		id    string
		title string
	}
	var pending []pendingRow
	for rows.Next() {
		var r pendingRow
		if err := rows.Scan(&r.id, &r.title); err != nil {
			return err
		}
		pending = append(pending, r)
	}
	if err := rows.Err(); err != nil {
		return err
	}
	if len(pending) == 0 {
		return nil
	}

	groupByBase := make(map[string]string)
	for _, r := range pending {
		groupID := r.id
		chunkIdx := 1
		chunkTot := 1

		if m := chunkTitleSuffixRegex.FindStringSubmatch(strings.TrimSpace(r.title)); len(m) == 4 {
			baseKey := strings.TrimSpace(m[1]) + "|" + m[3]
			if existingGroupID, ok := groupByBase[baseKey]; ok {
				groupID = existingGroupID
			} else {
				newGroup := uuid.New().String()
				groupByBase[baseKey] = newGroup
				groupID = newGroup
			}
			if idx, err := strconv.Atoi(m[2]); err == nil {
				chunkIdx = idx
			}
			if tot, err := strconv.Atoi(m[3]); err == nil {
				chunkTot = tot
			}
		}

		if _, err := conn.ExecContext(
			ctx,
			"UPDATE ideas SET group_id = $1, chunk_index = $2, chunk_total = $3 WHERE id = $4",
			groupID,
			chunkIdx,
			chunkTot,
			r.id,
		); err != nil {
			return err
		}
	}

	return nil
}

func EnsureDevelopmentUser(ctx context.Context, conn *sql.DB) error {
	const query = `
		INSERT INTO users (id, email, password_hash, role, auth_provider, email_verified, token_version)
		VALUES ($1, $2, $3, 'admin', 'local', 1, 1)
		ON CONFLICT (id) DO UPDATE SET email_verified = 1
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
	type migEntry struct {
		name    string
		content []byte
	}
	var migrations []migEntry

	if entries, err := migrationsFS.ReadDir("migrations"); err == nil && len(entries) > 0 {
		for _, entry := range entries {
			if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".sql") {
				if content, err := migrationsFS.ReadFile("migrations/" + entry.Name()); err == nil {
					migrations = append(migrations, migEntry{name: entry.Name(), content: content})
				}
			}
		}
	} else if entries, err := os.ReadDir(migrationsDir); err == nil {
		for _, entry := range entries {
			if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".sql") {
				if content, err := os.ReadFile(filepath.Join(migrationsDir, entry.Name())); err == nil {
					migrations = append(migrations, migEntry{name: entry.Name(), content: content})
				}
			}
		}
	}

	sort.Slice(migrations, func(i, j int) bool {
		return migrations[i].name < migrations[j].name
	})

	for _, mig := range migrations {
		var count int
		err := conn.QueryRowContext(
			ctx,
			"SELECT COUNT(1) FROM schema_migrations WHERE version = $1",
			mig.name,
		).Scan(&count)
		if err != nil {
			return fmt.Errorf("failed checking migration %s: %w", mig.name, err)
		}

		if count > 0 {
			continue
		}

		tx, err := conn.BeginTx(ctx, nil)
		if err != nil {
			return fmt.Errorf("failed starting transaction for %s: %w", mig.name, err)
		}

		if strings.TrimSpace(string(mig.content)) != "" {
			if _, err := tx.ExecContext(ctx, string(mig.content)); err != nil {
				_ = tx.Rollback()
				return fmt.Errorf("migration %s failed: %w", mig.name, err)
			}
		}

		if _, err := tx.ExecContext(
			ctx,
			"INSERT INTO schema_migrations (version) VALUES ($1)",
			mig.name,
		); err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("failed recording migration %s: %w", mig.name, err)
		}

		if err := tx.Commit(); err != nil {
			return fmt.Errorf("failed committing migration %s: %w", mig.name, err)
		}

		log.Printf("[migrations] Applied migration: %s", mig.name)
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
