-- name: CreateUser :one
INSERT INTO users (id, email, password_hash, role)
VALUES ($1, $2, $3, $4)
RETURNING *;

-- name: GetUserByEmail :one
SELECT * FROM users
WHERE email = $1 LIMIT 1;

-- name: ListUsers :many
SELECT id, email, created_at
FROM users
ORDER BY created_at DESC;

-- name: ListIdeas :many
SELECT *
FROM ideas
ORDER BY created_at DESC;

-- name: ListIdeasByUserID :many
SELECT *
FROM ideas
WHERE user_id = $1
ORDER BY created_at DESC;

-- name: CreateIdea :one
INSERT INTO ideas (id, user_id, title, content, status)
VALUES ($1, $2, $3, $4, $5)
RETURNING *;

-- name: UpdateIdeaAnalysis :one
UPDATE ideas
SET restructured_content = $2,
    embedding = $3,
    status = $4,
    updated_at = CURRENT_TIMESTAMP
WHERE id = $1
RETURNING *;

-- name: UpdateKeywordEmbedding :one
UPDATE keywords
SET embedding = $2
WHERE id = $1
RETURNING *;

-- name: ListIdeasWithEmbeddings :many
SELECT id, title, embedding
FROM ideas
WHERE id != $1 AND embedding != '';

-- name: UpsertKeyword :one
INSERT INTO keywords (id, name)
VALUES ($1, $2)
ON CONFLICT (name) DO UPDATE SET name = EXCLUDED.name
RETURNING *;

-- name: LinkIdeaKeyword :exec
INSERT INTO idea_keywords (idea_id, keyword_id)
VALUES ($1, $2)
ON CONFLICT (idea_id, keyword_id) DO NOTHING;

-- name: ListKeywordsByIdeaID :many
SELECT k.*
FROM keywords k
JOIN idea_keywords ik ON ik.keyword_id = k.id
WHERE ik.idea_id = $1
ORDER BY k.name ASC;

-- name: CreateIdeaLink :one
INSERT INTO idea_links (id, source_idea_id, target_idea_id, relation_type, weight)
VALUES ($1, $2, $3, $4, $5)
ON CONFLICT (source_idea_id, target_idea_id, relation_type)
DO UPDATE SET weight = EXCLUDED.weight
RETURNING *;

-- name: FindOverlappingIdeas :many
SELECT ik2.idea_id, COUNT(ik2.keyword_id) AS shared_count
FROM idea_keywords ik1
JOIN idea_keywords ik2 ON ik1.keyword_id = ik2.keyword_id
WHERE ik1.idea_id = $1 AND ik2.idea_id != $1
GROUP BY ik2.idea_id;

-- name: ListAllIdeaLinks :many
SELECT *
FROM idea_links
ORDER BY weight DESC;

-- name: CreateIdeaChunk :one
INSERT INTO idea_chunks (id, idea_id, chunk_index, content, embedding)
VALUES ($1, $2, $3, $4, $5)
RETURNING *;

-- name: DeleteIdea :exec
DELETE FROM ideas WHERE id = $1;
