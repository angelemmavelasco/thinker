-- name: CreateUser :one
INSERT INTO users (id, email, password_hash)
VALUES ($1, $2, $3)
RETURNING *;

-- name: GetUserByEmail :one
SELECT * FROM users
WHERE email = $1 LIMIT 1;

-- name: ListUsers :many
SELECT id, email, created_at
FROM users
ORDER BY created_at DESC;





-- name: ListIdeas :many
SELECT id, user_id, title, content, status, created_at, updated_at
FROM ideas
ORDER BY created_at DESC;

-- name: ListIdeasByUserID :many
SELECT id, user_id, title, content, status, created_at, updated_at
FROM ideas
WHERE user_id = $1
ORDER BY created_at DESC;

-- name: CreateIdea :one
INSERT INTO ideas (id, user_id, title, content, status)
VALUES ($1, $2, $3, $4, $5)
RETURNING *;