# Manual Test Plan

## Prerequisites

```bash
make docker-up          # starts postgres, minio, rabbitmq, migrate, server, worker
docker compose logs -f worker  # keep in a separate terminal to watch thumbnails
```

Verify all services are healthy:
```bash
docker compose ps       # all services should show "Up" or "Exited (0)" for migrate/minio-setup
```

---

## 1. Health Check

```bash
curl -s http://localhost:8080/health | jq .
```

**Expected:** HTTP 200, JSON with `"postgres": "ok"` and `"s3": "ok"`.

---

## 2. Upload Avatar (happy path)

```bash
curl -s -X POST http://localhost:8080/api/v1/avatars \
  -H "X-User-ID: user-1" \
  -F "file=@testdata/photo.jpg" | jq .
```

**Expected:** HTTP 201, JSON with `"id"`, `"user_id": "user-1"`, `"status": "pending"`.

Save the avatar ID:
```bash
AVATAR_ID=<paste id from response>
```

---

## 3. Verify Worker Processed Thumbnails

Check worker logs (in the separate terminal):
```
"processed upload event", "avatar_id": "<AVATAR_ID>"
```

Then verify via metadata endpoint:
```bash
curl -s http://localhost:8080/api/v1/avatars/$AVATAR_ID/metadata | jq .
```

**Expected:** JSON with `"thumbnails"` array containing entries for `"100x100"` and `"300x300"`.

---

## 4. Get Avatar Image (original + thumbnails)

```bash
# Original
curl -s -o original.jpg http://localhost:8080/api/v1/avatars/$AVATAR_ID
file original.jpg   # should say JPEG

# 100x100 thumbnail
curl -s -o thumb100.jpg "http://localhost:8080/api/v1/avatars/$AVATAR_ID?size=100x100"
identify thumb100.jpg  # 100x100 (requires imagemagick) or: file thumb100.jpg

# 300x300 thumbnail
curl -s -o thumb300.jpg "http://localhost:8080/api/v1/avatars/$AVATAR_ID?size=300x300"
```

**Expected:** Valid JPEG files at correct dimensions.

---

## 5. Get User's Latest Avatar

```bash
curl -s -o user_avatar.jpg http://localhost:8080/api/v1/users/user-1/avatar
file user_avatar.jpg
```

**Expected:** HTTP 200, valid image file.

---

## 6. List User Avatars

```bash
curl -s http://localhost:8080/api/v1/users/user-1/avatars | jq .
```

**Expected:** JSON array with at least 1 entry containing `"id"`, `"file_name"`, `"url"`.

---

## 7. Upload File Too Large (413)

```bash
# Create a >10MB file
dd if=/dev/zero of=/tmp/big.bin bs=1M count=11 2>/dev/null

curl -s -X POST http://localhost:8080/api/v1/avatars \
  -H "X-User-ID: user-1" \
  -F "file=@/tmp/big.bin" | jq .
```

**Expected:** HTTP 413, `{"error": "File too large", "max_size": 10485760}`.

---

## 8. Upload Without X-User-ID (400)

```bash
curl -s -X POST http://localhost:8080/api/v1/avatars \
  -F "file=@testdata/photo.jpg" | jq .
```

**Expected:** HTTP 400, `{"error": "X-User-ID header is required"}`.

---

## 9. Get Non-existent Avatar (404)

```bash
curl -s http://localhost:8080/api/v1/avatars/00000000-0000-0000-0000-000000000000/metadata | jq .
```

**Expected:** HTTP 404, `{"error": "avatar not found"}`.

---

## 10. Delete Avatar

```bash
curl -s -o /dev/null -w "%{http_code}" -X DELETE \
  http://localhost:8080/api/v1/avatars/$AVATAR_ID \
  -H "X-User-ID: user-1"
```

**Expected:** HTTP 204 (no body).

Verify deletion:
```bash
curl -s http://localhost:8080/api/v1/avatars/$AVATAR_ID/metadata | jq .
```

**Expected:** HTTP 404.

Check worker logs for delete event processing (S3 cleanup).

---

## 11. Delete Forbidden (wrong user)

Upload a new avatar first:
```bash
AVATAR_ID2=$(curl -s -X POST http://localhost:8080/api/v1/avatars \
  -H "X-User-ID: user-1" \
  -F "file=@testdata/photo.jpg" | jq -r .id)
```

Try deleting as another user:
```bash
curl -s -X DELETE http://localhost:8080/api/v1/avatars/$AVATAR_ID2 \
  -H "X-User-ID: user-999" | jq .
```

**Expected:** HTTP 403, `{"error": "you can only delete your own avatars"}`.

---

## 12. Delete User Avatar (by user_id)

```bash
curl -s -o /dev/null -w "%{http_code}" -X DELETE \
  http://localhost:8080/api/v1/users/user-1/avatar \
  -H "X-User-ID: user-1"
```

**Expected:** HTTP 204.

---

## 13. Idempotency Check

Upload an avatar and wait for worker to process it:
```bash
AVATAR_ID3=$(curl -s -X POST http://localhost:8080/api/v1/avatars \
  -H "X-User-ID: user-2" \
  -F "file=@testdata/photo.jpg" | jq -r .id)

sleep 3  # wait for worker

curl -s http://localhost:8080/api/v1/avatars/$AVATAR_ID3/metadata | jq .thumbnails
```

**Expected:** Thumbnails present (status = completed). If the message were redelivered, worker would skip processing (check worker logs for "already completed" or no second "processed upload event").

---

## 14. Web Interface

Open http://localhost:8080/ in browser.

1. Enter a User ID
2. Select an image file
3. Preview should appear
4. Click "Upload Avatar"
5. Response panel should show HTTP 201 with avatar JSON

---

## 15. MinIO Verification

Open http://localhost:9001/ (login: minioadmin/minioadmin).

Navigate to `avatars` bucket:
- `originals/` — should contain uploaded files
- `thumbnails/<avatar_id>/` — should contain `100x100.jpg` and `300x300.jpg`

After deletion, verify files are removed by the worker.

---

## Cleanup

```bash
make docker-down        # stop all services
make docker-clean       # remove volumes
rm -f original.jpg thumb100.jpg thumb300.jpg user_avatar.jpg /tmp/big.bin
```
