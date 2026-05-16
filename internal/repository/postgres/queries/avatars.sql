-- name: CreateAvatar :one
INSERT INTO avatars (id, user_id, file_name, mime_type, size_bytes, s3_key)
VALUES ($1, $2, $3, $4, $5, $6) RETURNING *;

-- name: SetAvatarUploaded :exec
UPDATE avatars
SET upload_status = 'uploaded',
    updated_at    = NOW()
WHERE id = $1
  AND deleted_at IS NULL;

-- name: SetAvatarUploadFailed :exec
UPDATE avatars
SET upload_status = 'failed',
    updated_at    = NOW()
WHERE id = $1
  AND deleted_at IS NULL;

-- name: GetAvatarByID :one
SELECT *
FROM avatars
WHERE id = $1
  AND deleted_at IS NULL;

-- name: GetLatestAvatarByUserID :one
SELECT *
FROM avatars
WHERE user_id = $1
  AND deleted_at IS NULL
ORDER BY created_at DESC LIMIT 1;

-- name: GetAvatarsByUserID :many
SELECT *
FROM avatars
WHERE user_id = $1
  AND deleted_at IS NULL
ORDER BY created_at DESC;

-- name: SoftDeleteAvatar :execrows
UPDATE avatars
SET deleted_at = NOW(),
    updated_at = NOW()
WHERE id = $1
  AND user_id = $2
  AND deleted_at IS NULL;

-- name: GetProcessingStatus :one
SELECT processing_status
FROM avatars
WHERE id = $1
  AND deleted_at IS NULL;

-- name: UpdateProcessingStatus :exec
UPDATE avatars
SET processing_status = $2,
    thumbnail_s3_keys = $3,
    updated_at        = NOW()
WHERE id = $1
  AND deleted_at IS NULL;

-- name: GetTotalStorageBytes :one
SELECT COALESCE(SUM(size_bytes), 0)::bigint AS total
FROM avatars
WHERE deleted_at IS NULL;
