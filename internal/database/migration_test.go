package database

import (
	"context"
	"database/sql"
	"testing"

	"github.com/google/uuid"
	_ "modernc.org/sqlite"
)

func TestInitSchema_And_BackfillIdeaGroups(t *testing.T) {
	ctx := context.Background()

	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("failed opening in-memory db: %v", err)
	}
	defer db.Close()

	if err := InitSchema(ctx, db); err != nil {
		t.Fatalf("InitSchema failed: %v", err)
	}

	// Insert ideas requiring backfill: one with nil UUID, one with empty text
	userID := uuid.New().String()
	_, err = db.ExecContext(ctx, `
		INSERT INTO users (id, email, password_hash, role, auth_provider, email_verified, token_version)
		VALUES ($1, 'test@test.local', 'hash', 'user', 'local', 1, 1);
	`, userID)
	if err != nil {
		t.Fatalf("failed inserting test user: %v", err)
	}

	idea1ID := uuid.New().String()
	idea2ID := uuid.New().String()
	_, err = db.ExecContext(ctx, `
		INSERT INTO ideas (id, user_id, group_id, chunk_index, chunk_total, title, content, status)
		VALUES 
			($1, $2, '00000000-0000-0000-0000-000000000000', 0, 0, 'Document A (1/2)', 'Content 1', 'pendiente'),
			($3, $2, '', 0, 0, 'Document A (2/2)', 'Content 2', 'pendiente');
	`, idea1ID, userID, idea2ID)
	if err != nil {
		t.Fatalf("failed inserting test ideas: %v", err)
	}

	// Run backfillIdeaGroups
	if err := backfillIdeaGroups(ctx, db); err != nil {
		t.Fatalf("backfillIdeaGroups failed: %v", err)
	}

	// Verify both chunks got assigned the same new group_id
	var group1, group2 string
	var idx1, idx2, tot1, tot2 int
	err = db.QueryRowContext(ctx, "SELECT group_id, chunk_index, chunk_total FROM ideas WHERE id = $1", idea1ID).Scan(&group1, &idx1, &tot1)
	if err != nil {
		t.Fatalf("failed querying idea1: %v", err)
	}
	err = db.QueryRowContext(ctx, "SELECT group_id, chunk_index, chunk_total FROM ideas WHERE id = $1", idea2ID).Scan(&group2, &idx2, &tot2)
	if err != nil {
		t.Fatalf("failed querying idea2: %v", err)
	}

	if group1 == "00000000-0000-0000-0000-000000000000" || group1 == "" {
		t.Errorf("idea1 group_id was not backfilled: %s", group1)
	}
	if group1 != group2 {
		t.Errorf("expected both chunks of Document A to share group_id, got %s and %s", group1, group2)
	}
	if idx1 != 1 || tot1 != 2 || idx2 != 2 || tot2 != 2 {
		t.Errorf("chunk index/total mismatch: (%d/%d), (%d/%d)", idx1, tot1, idx2, tot2)
	}
}
