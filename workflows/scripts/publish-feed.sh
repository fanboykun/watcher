#!/usr/bin/env bash
# Publishes one service release into a Watcher release feed on MinIO or S3,
# using the MinIO client (mc). Layout and ordering follow
# docs/adr/0002-internal-release-feed.md in the Watcher repository:
#
#   <feed>/releases/<tag>/<zip>            1. artifact (write-once)
#   <feed>/releases/<tag>/version.json     2. per-release manifest (write-once)
#   <feed>/releases.json                   3. release index, newest first
#   <feed>/version.json                    4. latest pointer, written LAST
#
# Required environment:
#   FEED_TARGET    mc path of the feed root: <alias>/<bucket>/<prefix>
#   SERVICE        service key Watcher matches against its service_name
#   VERSION        version Watcher compares (monorepos usually pass the tag)
#   TAG            release tag; becomes the directory under releases/
#   ZIP_PATH       the built artifact
# Optional environment:
#   SERVICE_HINTS  JSON object of deployment hints (app_kind, binary_name, ...)
#   RELEASE_NAME   display title for the release catalog
#   NOTES          release notes for the release catalog
#   PRERELEASE     "true" indexes the release without moving the latest pointer
#
# Credentials come from mc's own configuration: either an alias configured on
# the runner host, or an MC_HOST_<alias> environment variable.
set -euo pipefail

: "${FEED_TARGET:?FEED_TARGET is required}" "${SERVICE:?SERVICE is required}" "${VERSION:?VERSION is required}"
: "${TAG:?TAG is required}" "${ZIP_PATH:?ZIP_PATH is required}"
SERVICE_HINTS="${SERVICE_HINTS:-}"
[ -n "$SERVICE_HINTS" ] || SERVICE_HINTS='{}'
PRERELEASE="${PRERELEASE:-false}"
FEED_TARGET="${FEED_TARGET%/}"

fail() { echo "::error::$*" >&2; exit 1; }

for tool in mc jq sha256sum; do
  command -v "$tool" >/dev/null || fail "$tool must be installed on the release runner"
done
[ -f "$ZIP_PATH" ] || fail "artifact $ZIP_PATH does not exist"
jq -e 'type == "object"' >/dev/null <<<"$SERVICE_HINTS" || fail "SERVICE_HINTS must be a JSON object"
# Tags become directories; Watcher rejects the same refs when pinning.
if [[ ! "$TAG" =~ ^[A-Za-z0-9._+-]+(/[A-Za-z0-9._+-]+)*$ ]] || [[ "/$TAG/" == *"/../"* || "/$TAG/" == *"/./"* ]]; then
  fail "tag '$TAG' cannot name a feed directory"
fi

# Reading must not be confused with absence: a missing index is created, but a
# credential or network error must never overwrite the feed's index.
mc ls "$FEED_TARGET/" >/dev/null || fail "cannot list $FEED_TARGET; check the mc alias, bucket and credentials"
read_or_default() { # <object> <default json> <output file>
  local out
  if out="$(mc cat "$1" 2>&1)"; then
    printf '%s\n' "$out" >"$3"
  elif grep -qiE 'does not exist|not found|NoSuchKey' <<<"$out"; then
    printf '%s\n' "$2" >"$3"
  else
    fail "cannot read $1: $out"
  fi
  jq -e . "$3" >/dev/null || fail "$1 is not valid JSON"
}

ZIP_NAME="$(basename "$ZIP_PATH")"
RELEASE_DIR="$FEED_TARGET/releases/$TAG"
for object in "$RELEASE_DIR/$ZIP_NAME" "$RELEASE_DIR/version.json"; do
  if mc stat "$object" >/dev/null 2>&1; then
    fail "$object already exists; feed releases are write-once, so bump the version instead"
  fi
done

SHA256="$(sha256sum "$ZIP_PATH" | cut -d' ' -f1)"
SIZE="$(wc -c <"$ZIP_PATH" | tr -d ' ')"
PUBLISHED_AT="$(date -u +%Y-%m-%dT%H:%M:%SZ)"
uri_path() { jq -rn --arg p "$1" '$p | split("/") | map(@uri) | join("/")'; }
ZIP_URI="$(uri_path "$ZIP_NAME")"
ROOT_ARTIFACT_URL="releases/$(uri_path "$TAG")/$ZIP_URI"

WORK="$(mktemp -d)"
trap 'rm -rf "$WORK"' EXIT
ENTRY="$(jq -c -n \
  --argjson hints "$SERVICE_HINTS" --arg version "$VERSION" --arg artifact "$ZIP_NAME" \
  --arg sha "$SHA256" --arg size "$SIZE" --arg at "$PUBLISHED_AT" \
  '$hints + {version: $version, artifact: $artifact, sha256: $sha, size: ($size | tonumber), published_at: $at}')"

# Per-release manifest: its artifact sits beside it.
jq -n --arg svc "$SERVICE" --argjson entry "$ENTRY" --arg url "$ZIP_URI" \
  '{services: {($svc): ($entry + {artifact_url: $url})}}' >"$WORK/release.json"

read_or_default "$FEED_TARGET/releases.json" '{"releases":[]}' "$WORK/index.json"
jq --arg tag "$TAG" --arg name "${RELEASE_NAME:-}" --arg notes "${NOTES:-}" --arg at "$PUBLISHED_AT" \
  --arg svc "$SERVICE" --argjson entry "$ENTRY" --arg url "$ROOT_ARTIFACT_URL" --argjson pre "$PRERELEASE" \
  '.releases = ([{tag: $tag, name: $name, published_at: $at, prerelease: $pre, notes: $notes,
                  services: {($svc): ($entry + {artifact_url: $url})}}]
                + [(.releases // [])[] | select(.tag != $tag)])' \
  "$WORK/index.json" >"$WORK/index.next.json"

# The latest pointer keeps every other service's entry, so monorepo services share a feed root.
read_or_default "$FEED_TARGET/version.json" '{"services":{}}' "$WORK/latest.json"
jq --arg svc "$SERVICE" --argjson entry "$ENTRY" --arg url "$ROOT_ARTIFACT_URL" \
  '.services = ((.services // {}) + {($svc): ($entry + {artifact_url: $url})})' \
  "$WORK/latest.json" >"$WORK/latest.next.json"

echo "Publishing $SERVICE $VERSION ($TAG) to $FEED_TARGET"
mc cp --quiet "$ZIP_PATH" "$RELEASE_DIR/$ZIP_NAME"
mc cp --quiet "$WORK/release.json" "$RELEASE_DIR/version.json"
mc cp --quiet "$WORK/index.next.json" "$FEED_TARGET/releases.json"
if [ "$PRERELEASE" = "true" ]; then
  echo "Prerelease: latest pointer unchanged"
else
  mc cp --quiet "$WORK/latest.next.json" "$FEED_TARGET/version.json"
fi

{
  echo "sha256=$SHA256"
  echo "artifact_path=$ROOT_ARTIFACT_URL"
} >>"${GITHUB_OUTPUT:-/dev/null}"
echo "Published $ROOT_ARTIFACT_URL (sha256 $SHA256)"
