-- name: CreateUser :one
INSERT INTO users (id, email, password_hash, role, auth_provider, google_id, email_verified, token_version)
VALUES ($1, $2, $3, $4, $5, $6, $7, 1)
RETURNING *;

-- name: GetUserByID :one
SELECT * FROM users
WHERE id = $1 LIMIT 1;

-- name: GetUserByEmail :one
SELECT * FROM users
WHERE LOWER(email) = LOWER($1) LIMIT 1;

-- name: GetUserByGoogleID :one
SELECT * FROM users
WHERE google_id = $1 AND google_id != '' LIMIT 1;

-- name: ListUsers :many
SELECT id, email, role, auth_provider, email_verified, created_at
FROM users
ORDER BY created_at DESC;

-- name: LinkGoogleAccount :one
UPDATE users
SET google_id = $2,
    email_verified = 1
WHERE id = $1
RETURNING *;

-- name: MarkUserEmailVerified :one
UPDATE users
SET email_verified = 1
WHERE id = $1
RETURNING *;

-- name: RecordFailedLoginAttempt :one
UPDATE users
SET failed_login_attempts = $2,
    locked_until = $3
WHERE id = $1
RETURNING *;

-- name: ResetFailedLoginAttempts :exec
UPDATE users
SET failed_login_attempts = 0,
    locked_until = NULL
WHERE id = $1;

-- name: UpdateUserPasswordAndBumpTokenVersion :one
UPDATE users
SET password_hash = $2,
    token_version = token_version + 1,
    failed_login_attempts = 0,
    locked_until = NULL
WHERE id = $1
RETURNING *;

-- name: IncrementUserTokenVersion :one
UPDATE users
SET token_version = token_version + 1
WHERE id = $1
RETURNING token_version;

-- name: CreateRefreshToken :one
INSERT INTO refresh_tokens (
    id, user_id, token_hash, family_id, revoked, user_agent, ip_address, expires_at
)
VALUES ($1, $2, $3, $4, 0, $5, $6, $7)
RETURNING *;

-- name: GetRefreshTokenByHash :one
SELECT * FROM refresh_tokens
WHERE token_hash = $1 LIMIT 1;

-- name: MarkRefreshTokenRotated :exec
UPDATE refresh_tokens
SET revoked = 1,
    replaced_by_id = $2,
    revoked_at = $3
WHERE id = $1 AND revoked = 0;

-- name: RevokeRefreshTokenFamily :exec
UPDATE refresh_tokens
SET revoked = 1,
    revoked_at = $2
WHERE family_id = $1 AND revoked = 0;

-- name: RevokeAllUserRefreshTokens :exec
UPDATE refresh_tokens
SET revoked = 1,
    revoked_at = $2
WHERE user_id = $1 AND revoked = 0;

-- name: DeleteExpiredRefreshTokens :exec
DELETE FROM refresh_tokens
WHERE expires_at < $1;

-- name: CreateActionToken :one
INSERT INTO action_tokens (
    id, user_id, token_hash, token_type, expires_at
)
VALUES ($1, $2, $3, $4, $5)
RETURNING *;

-- name: GetActionTokenByHash :one
SELECT * FROM action_tokens
WHERE token_hash = $1 AND token_type = $2 LIMIT 1;

-- name: ConsumeActionToken :one
UPDATE action_tokens
SET consumed_at = $2
WHERE id = $1 AND consumed_at IS NULL
RETURNING *;

-- name: InvalidateUserActionTokensByType :exec
UPDATE action_tokens
SET consumed_at = $3
WHERE user_id = $1 AND token_type = $2 AND consumed_at IS NULL;

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

-- name: CreateKnowledgeMOC :one
INSERT INTO knowledge_mocs (id, code, title, description, embedding, display_order, is_locked)
VALUES ($1, $2, $3, $4, $5, $6, $7)
RETURNING *;

-- name: GetKnowledgeMOCByID :one
SELECT * FROM knowledge_mocs
WHERE id = $1 LIMIT 1;

-- name: GetKnowledgeMOCByCode :one
SELECT * FROM knowledge_mocs
WHERE UPPER(code) = UPPER($1) LIMIT 1;

-- name: ListKnowledgeMOCs :many
SELECT * FROM knowledge_mocs
ORDER BY display_order ASC, created_at ASC;

-- name: UpdateKnowledgeMOC :one
UPDATE knowledge_mocs
SET title = $2,
    description = $3,
    embedding = $4,
    display_order = $5,
    is_locked = $6,
    updated_at = CURRENT_TIMESTAMP
WHERE id = $1
RETURNING *;

-- name: UpdateKnowledgeMOCLock :one
UPDATE knowledge_mocs
SET is_locked = $2,
    updated_at = CURRENT_TIMESTAMP
WHERE id = $1
RETURNING *;

-- name: DeleteKnowledgeMOC :exec
DELETE FROM knowledge_mocs
WHERE id = $1;

-- name: CountKnowledgeMOCs :one
SELECT COUNT(*) FROM knowledge_mocs;

-- name: CreateKnowledgeHub :one
INSERT INTO knowledge_hubs (id, moc_id, title, summary, search_aliases, embedding, display_order, is_locked)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
RETURNING *;

-- name: GetKnowledgeHubByID :one
SELECT * FROM knowledge_hubs
WHERE id = $1 LIMIT 1;

-- name: ListKnowledgeHubsByMOCID :many
SELECT * FROM knowledge_hubs
WHERE moc_id = $1
ORDER BY display_order ASC, created_at ASC;

-- name: ListAllKnowledgeHubs :many
SELECT * FROM knowledge_hubs
ORDER BY display_order ASC, created_at ASC;

-- name: UpdateKnowledgeHub :one
UPDATE knowledge_hubs
SET moc_id = $2,
    title = $3,
    summary = $4,
    search_aliases = $5,
    embedding = $6,
    display_order = $7,
    is_locked = $8,
    updated_at = CURRENT_TIMESTAMP
WHERE id = $1
RETURNING *;

-- name: UpdateKnowledgeHubLock :one
UPDATE knowledge_hubs
SET is_locked = $2,
    updated_at = CURRENT_TIMESTAMP
WHERE id = $1
RETURNING *;

-- name: DeleteKnowledgeHub :exec
DELETE FROM knowledge_hubs
WHERE id = $1;

-- name: CountKnowledgeHubsByMOCID :one
SELECT COUNT(*) FROM knowledge_hubs
WHERE moc_id = $1;

-- name: CreateHubIdeaEdge :one
INSERT INTO hub_idea_edges (hub_id, idea_id, relationship_type, weight, is_manual_override)
VALUES ($1, $2, $3, $4, $5)
ON CONFLICT (hub_id, idea_id)
DO UPDATE SET relationship_type = EXCLUDED.relationship_type,
              weight = EXCLUDED.weight,
              is_manual_override = EXCLUDED.is_manual_override
RETURNING *;

-- name: ListIdeasByHubID :many
SELECT i.*, hie.relationship_type, hie.weight, hie.is_manual_override
FROM ideas i
JOIN hub_idea_edges hie ON hie.idea_id = i.id
WHERE hie.hub_id = $1
ORDER BY hie.weight DESC;

-- name: ListHubsByIdeaID :many
SELECT kh.*, hie.relationship_type, hie.weight, hie.is_manual_override
FROM knowledge_hubs kh
JOIN hub_idea_edges hie ON hie.hub_id = kh.id
WHERE hie.idea_id = $1
ORDER BY hie.weight DESC;

-- name: DeleteHubIdeaEdge :exec
DELETE FROM hub_idea_edges
WHERE hub_id = $1 AND idea_id = $2;

-- name: DeleteHubIdeaEdgesByIdeaID :exec
DELETE FROM hub_idea_edges
WHERE idea_id = $1;

-- name: DeleteHubIdeaEdgesByHubID :exec
DELETE FROM hub_idea_edges
WHERE hub_id = $1;

-- name: ListAllHubIdeaEdges :many
SELECT * FROM hub_idea_edges
ORDER BY weight DESC;

-- name: GetKnowledgeSetting :one
SELECT * FROM knowledge_settings
WHERE key = $1 LIMIT 1;

-- name: UpsertKnowledgeSetting :one
INSERT INTO knowledge_settings (key, value)
VALUES ($1, $2)
ON CONFLICT (key) DO UPDATE SET value = EXCLUDED.value, updated_at = CURRENT_TIMESTAMP
RETURNING *;

-- name: ListKnowledgeSettings :many
SELECT * FROM knowledge_settings
ORDER BY key ASC;
