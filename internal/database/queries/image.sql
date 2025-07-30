-- name: CreateImageTable :exec
CREATE TABLE IF NOT EXISTS images (
    id bigint GENERATED ALWAYS AS IDENTITY,
    name VARCHAR(253) NOT NULL,
    namespace VARCHAR(253) NOT NULL,
    object TEXT NOT NULL,
    UNIQUE(name, namespace)
);

-- name: CreateImage :execresult
INSERT INTO images (
  name, namespace, object
) VALUES (
  $1, $2, $3
)
ON CONFLICT DO NOTHING;

-- name: DeleteImage :one
DELETE FROM images
WHERE name = $1 AND namespace = $2
RETURNING *;

-- name: GetImage :one
SELECT * FROM images
WHERE name = $1 AND namespace = $2;

-- name: ListImagesByNamespace :many
SELECT * FROM images WHERE namespace = $1;

-- name: ListImages :many
SELECT * FROM images;

-- name: UpdateImage :exec
UPDATE images
SET object = $1
WHERE name = $2 AND namespace = $3;

-- name: CountImagesByNamespace :one
SELECT COUNT(*) FROM images WHERE namespace = $1;

-- name: CountImages :one
SELECT COUNT(*) FROM images;