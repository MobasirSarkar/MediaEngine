-- name: CreateUpload :one
INSERT INTO uploads (id, owner_id, filename, content_type, total_size, chunk_size, total_chunks, status, bucket, key, expires_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
RETURNING *;

-- name: GetUpload :one
SELECT * FROM uploads WHERE id = $1;

-- name: UpdateUploadStatus :one
UPDATE uploads SET status = $2, updated_at = now() WHERE id = $1 RETURNING *;

-- name: IncrementReceivedBytes :one
UPDATE uploads SET received_bytes = received_bytes + $2, updated_at = now()
WHERE id = $1 RETURNING *;

-- name: SetUploadChecksum :one
UPDATE uploads SET checksum_sha256 = $2, status = $3, updated_at = now()
WHERE id = $1 RETURNING *;

-- name: ListExpiredUploads :many
SELECT * FROM uploads WHERE status = 'uploading' AND expires_at < now() FOR UPDATE SKIP LOCKED;

-- name: AppendChunk :one
INSERT INTO chunks (upload_id, chunk_no, size, checksum)
VALUES ($1, $2, $3, $4)
ON CONFLICT (upload_id, chunk_no) DO UPDATE
    SET size = EXCLUDED.size, checksum = EXCLUDED.checksum, received_at = now()
RETURNING *;

-- name: ListChunks :many
SELECT * FROM chunks WHERE upload_id = $1 ORDER BY chunk_no;

-- name: CountChunks :one
SELECT COUNT(*) FROM chunks WHERE upload_id = $1;

-- name: CreateJob :one
INSERT INTO jobs (id, upload_id, status) VALUES ($1, $2, $3) RETURNING *;

-- name: GetJob :one
SELECT * FROM jobs WHERE id = $1;

-- name: GetJobByUpload :one
SELECT * FROM jobs WHERE upload_id = $1;

-- name: UpdateJobStatus :one
UPDATE jobs SET status = $2, error_code = $3, error_msg = $4, retries = $5, updated_at = now()
WHERE id = $1 RETURNING *;

-- name: CreateTask :one
INSERT INTO tasks (id, job_id, kind, status, payload) VALUES ($1, $2, $3, $4, $5) RETURNING *;

-- name: GetTask :one
SELECT * FROM tasks WHERE id = $1;

-- name: ListTasksByJob :many
SELECT * FROM tasks WHERE job_id = $1 ORDER BY created_at;

-- name: UpdateTaskStarted :one
UPDATE tasks SET status = 'processing', attempt = attempt + 1, started_at = now(), updated_at = now()
WHERE id = $1 RETURNING *;

-- name: UpdateTaskCompleted :one
UPDATE tasks SET status = 'completed', result = $2, finished_at = now(), updated_at = now()
WHERE id = $1 RETURNING *;

-- name: UpdateTaskFailed :one
UPDATE tasks SET status = 'retrying', error_msg = $2, updated_at = now()
WHERE id = $1 RETURNING *;
