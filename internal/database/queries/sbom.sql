-- name: CreateSbomTable :exec
CREATE TABLE IF NOT EXISTS sboms (
    id bigint GENERATED ALWAYS AS IDENTITY,
    name VARCHAR(253) NOT NULL,
    namespace VARCHAR(253) NOT NULL,
    object TEXT NOT NULL,
    UNIQUE(name, namespace)
);

-- name: CreateSbom :execresult
INSERT INTO sboms (
  name, namespace, object
) VALUES (
  $1, $2, $3
)
ON CONFLICT DO NOTHING;

-- name: DeleteSbom :one
DELETE FROM sboms
WHERE name = $1 AND namespace = $2
RETURNING *;

-- name: GetSbom :one
SELECT * FROM sboms
WHERE name = $1 AND namespace = $2;

-- name: ListSbomByNamespace :many
SELECT * FROM sboms WHERE namespace = $1;

-- name: ListSbom :many
SELECT * FROM sboms;

-- name: UpdateSbom :exec
UPDATE sboms
SET object = $1
WHERE name = $2 AND namespace = $3;

-- name: CountSbomByNamespace :one
SELECT COUNT(*) FROM sboms WHERE namespace = $1;

-- name: CountSbom :one
SELECT COUNT(*) FROM sboms;