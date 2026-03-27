-- name: CreateAvatar :one
INSERT INTO avatars (id, user_id, file_name, mime_type, size_bytes, s3_key, upload_status, processing_status)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8) RETURNING *;
