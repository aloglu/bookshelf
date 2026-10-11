#!/usr/bin/env bash
# Runs a Bookshelf image through its life: start, health, sign-in, a book with
# a cover, a backup, a restart that keeps everything, the data folder owned by
# PUID:PGID, and a clean stop.
#
#   docker/smoke-test.sh <image>
#
# Needs docker, curl and python3. Leaves nothing behind.
set -euo pipefail

IMAGE="${1:?Usage: docker/smoke-test.sh <image>}"
PORT="${SMOKE_PORT:-18199}"
PASSWORD="smoke-test-password"
NAME="bookshelf-smoke-$$"
# The container gives the data folder to its own user, so it lives inside a
# folder of ours that we can always delete afterwards.
WORK="$(mktemp -d)"
DATA="$WORK/data"
mkdir "$DATA"
JAR="$WORK/cookies"
BASE="http://127.0.0.1:$PORT"

cleanup() {
    docker rm -f "$NAME" >/dev/null 2>&1 || true
    docker run --rm -v "$DATA:/data" --entrypoint sh "$IMAGE" -c 'rm -rf /data/* /data/.[!.]*' >/dev/null 2>&1 || true
    rm -rf "$WORK" || true
}
trap cleanup EXIT

step() { printf '\n== %s\n' "$*"; }
fail() { printf 'FAILED: %s\n' "$*" >&2; docker logs "$NAME" >&2 2>&1 || true; exit 1; }
json() { python3 -c "import json,sys; d=json.load(sys.stdin); print($1)"; }
api() { curl -sf -b "$JAR" -H "Origin: $BASE" "$@"; }

start() {
    docker run -d --name "$NAME" -p "127.0.0.1:$PORT:8080" -e BOOKSHELF_ADMIN_PASSWORD="$PASSWORD" \
        -e PUID=1234 -e PGID=1234 -v "$DATA:/data" "$IMAGE" >/dev/null
}
wait_healthy() {
    for _ in $(seq 1 60); do
        case "$(docker inspect -f '{{.State.Health.Status}}' "$NAME" 2>/dev/null)" in
            healthy) return 0 ;;
            unhealthy) fail "the container reports unhealthy" ;;
        esac
        sleep 1
    done
    fail "not healthy after 60 seconds"
}

step "Refusals"
# Both must stop at once with a plain message (and a failing exit, which the pipe ignores).
{ docker run --rm "$IMAGE" 2>&1 || true; } | grep -q "set BOOKSHELF_ADMIN_PASSWORD" || fail "started without a password"
{ docker run --rm -e BOOKSHELF_ADMIN_PASSWORD=x -e PUID=0 "$IMAGE" 2>&1 || true; } | grep -q "must not be 0" || fail "agreed to run as root"

step "Starting $IMAGE"
start
wait_healthy
curl -sf "$BASE/" | grep -q '<title>Bookshelf</title>' || fail "the page isn't served"
curl -sf "$BASE/978-0-679-72342-4" | grep -q '<title>Bookshelf</title>' || fail "a book's address isn't the app"

step "Signing in"
curl -sf -c "$JAR" -H "Origin: $BASE" -H 'Content-Type: application/json' \
    -d "{\"user\":\"admin\",\"password\":\"$PASSWORD\"}" "$BASE/api/session" >/dev/null || fail "could not sign in"

step "A book with a cover"
id="$(api -H 'Content-Type: application/json' -d '{"title":"Pale Fire","author":"Vladimir Nabokov","identifier":"9780679723424","pages":315}' "$BASE/api/books" | json 'd["id"]')"
python3 -c "
import struct, zlib
w, h = 60, 90
raw = b''.join(b'\x00' + bytes([150, 40, 40]) * w for _ in range(h))
chunk = lambda t, d: struct.pack('>I', len(d)) + t + d + struct.pack('>I', zlib.crc32(t + d))
open('$WORK/cover.png', 'wb').write(b'\x89PNG\r\n\x1a\n' + chunk(b'IHDR', struct.pack('>IIBBBBB', w, h, 8, 2, 0, 0, 0)) + chunk(b'IDAT', zlib.compress(raw)) + chunk(b'IEND', b''))"
api -X PUT -H 'Content-Type: image/png' --data-binary "@$WORK/cover.png" "$BASE/api/books/$id/cover" | json 'd["spine"]["color"]' | grep -q '^#' || fail "the cover wasn't taken"
cover="$(curl -sf "$BASE/api/library" | json 'd["books"][0]["cover"]["url"]')"
curl -sf -o /dev/null "$BASE$cover" || fail "the cover isn't served"

step "A backup"
api -X POST "$BASE/api/backups" | json 'd["books"]' | grep -q '^1$' || fail "the backup doesn't hold the book"

step "Restarting keeps everything"
docker restart "$NAME" >/dev/null
wait_healthy
curl -sf "$BASE/api/library" | json 'd["books"][0]["title"]' | grep -q '^Pale Fire$' || fail "the book is gone after a restart"
api "$BASE/api/backups" | json 'len(d)' | grep -q '^[1-9]' || fail "the backups are gone after a restart"

step "The data belongs to PUID:PGID"
owners="$(docker run --rm -v "$DATA:/data" --entrypoint sh "$IMAGE" -c 'find /data -exec stat -c "%u:%g" {} + | sort -u')"
[ "$owners" = "1234:1234" ] || fail "data folder owners: $owners"
docker exec "$NAME" sh -c 'awk "/^Uid:/ {print \$2}" /proc/1/status' | grep -q '^1234$' || fail "the program doesn't run as PUID"

step "Stopping"
docker stop -t 10 "$NAME" >/dev/null
[ "$(docker inspect -f '{{.State.ExitCode}}' "$NAME")" = "0" ] || fail "didn't stop cleanly"

printf '\nAll smoke tests passed.\n'
