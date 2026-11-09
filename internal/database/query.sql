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
INSERT INTO ideas (id, user_id, group_id, chunk_index, chunk_total, title, content, status)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
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

-- name: GetIdeaByID :one
SELECT *
FROM ideas
WHERE id = $1
LIMIT 1;

-- name: ListDirectConnectedIdeas :many
SELECT
    i.id,
    i.title,
    i.content,
    i.restructured_content,
    i.status,
    il.relation_type,
    il.weight
FROM idea_links il
JOIN ideas i ON i.id = il.target_idea_id
WHERE il.source_idea_id = $1
ORDER BY il.weight DESC;

-- name: UpdateIdeaContentAndReset :one
UPDATE ideas
SET title = $2,
    content = $3,
    restructured_content = '',
    embedding = '',
    status = 'borrador',
    updated_at = CURRENT_TIMESTAMP
WHERE id = $1
RETURNING *;

-- name: ResetIdeaForReanalysis :one
UPDATE ideas
SET restructured_content = '',
    embedding = '',
    status = 'borrador',
    updated_at = CURRENT_TIMESTAMP
WHERE id = $1
RETURNING *;

-- name: DeleteIdeaLinksByIdeaID :exec
DELETE FROM idea_links
WHERE source_idea_id = $1 OR target_idea_id = $1;

-- name: DeleteIdeaKeywordsByIdeaID :exec
DELETE FROM idea_keywords
WHERE idea_id = $1;

-- name: DeleteOrphanKeywords :exec
DELETE FROM keywords
WHERE id NOT IN (SELECT DISTINCT keyword_id FROM idea_keywords);

-- name: DeleteIdeasByGroupID :exec
DELETE FROM ideas
WHERE group_id = $1;

-- name: ListIdeasByGroupID :many
SELECT id, title, chunk_index, chunk_total, status
FROM ideas
WHERE group_id = $1
ORDER BY chunk_index ASC, created_at ASC;
