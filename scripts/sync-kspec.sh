#!/usr/bin/env bash
#
# sync-kspec.sh — sync the vendored kspec content into internal/kspec/embed.
#
# This script is the ONLY entry point for content in internal/kspec/embed/.
# Never edit vendored files by hand: content changes happen in the upstream
# kspec repository (https://github.com/K77-dev/kspec) and land here by
# re-running this script.
#
# Usage:
#   scripts/sync-kspec.sh [ref] [source]
#
#   ref     git ref to sync (tag, branch or commit SHA).
#           Defaults to $KSPEC_REF or "v1.5.0" — the tag currently vendored.
#           The default is pinned so that re-running the script with no
#           arguments reproduces the committed embed tree; pass "main"
#           explicitly to track the upstream tip.
#   source  git repository URL or local path to clone from.
#           Defaults to $KSPEC_REPO or "https://github.com/K77-dev/kspec".
#           Until the v1.5.0 tag is pushed upstream, reproduce the vendored
#           tree from a local checkout: scripts/sync-kspec.sh v1.5.0 ../kspec
#
# Examples:
#   scripts/sync-kspec.sh                     # pinned v1.5.0 from GitHub
#   scripts/sync-kspec.sh main                # latest main from GitHub
#   KSPEC_REF=main scripts/sync-kspec.sh      # same, via env
#   scripts/sync-kspec.sh v1.5.0 ../kspec     # sync from a local checkout
#
# Third-party skills (anything not matching kspec-*) and skills-lock.json
# are excluded: only skills/kspec-*/, templates/*.md, rules/*.md and VERSION
# are vendored.

set -euo pipefail

DEFAULT_REF="v1.5.0"
DEFAULT_REPO="https://github.com/K77-dev/kspec"

REF="${1:-${KSPEC_REF:-$DEFAULT_REF}}"
SOURCE="${2:-${KSPEC_REPO:-$DEFAULT_REPO}}"

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
EMBED_DIR="$ROOT/internal/kspec/embed"

if [ -d "$SOURCE" ]; then
    SOURCE="file://$(cd "$SOURCE" && pwd)"
fi

TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT

CLONE_ERR="$TMP/clone.err"

if ! git clone --quiet --depth 1 --branch "$REF" "$SOURCE" "$TMP/src" 2>"$CLONE_ERR"; then
    rm -rf "$TMP/src"
    if ! grep -q "not found in upstream" "$CLONE_ERR"; then
        echo "sync-kspec: clone $SOURCE failed:" >&2
        cat "$CLONE_ERR" >&2
        exit 1
    fi
    if ! git clone --quiet "$SOURCE" "$TMP/src" 2>"$CLONE_ERR"; then
        echo "sync-kspec: clone $SOURCE failed:" >&2
        cat "$CLONE_ERR" >&2
        exit 1
    fi
    if ! git -C "$TMP/src" checkout --quiet --detach "$REF" 2>"$CLONE_ERR"; then
        echo "sync-kspec: checkout $REF from $SOURCE failed:" >&2
        cat "$CLONE_ERR" >&2
        exit 1
    fi
fi

for dir in skills templates rules; do
    if [ ! -d "$TMP/src/.agents/$dir" ]; then
        echo "sync-kspec: missing .agents/$dir in $SOURCE@$REF" >&2
        exit 1
    fi
done
if [ ! -f "$TMP/src/VERSION" ]; then
    echo "sync-kspec: missing VERSION in $SOURCE@$REF" >&2
    exit 1
fi

SKILL_DIRS=("$TMP/src/.agents/skills/"kspec-*)
if [ ! -d "${SKILL_DIRS[0]}" ]; then
    echo "sync-kspec: no kspec-* skills in $SOURCE@$REF" >&2
    exit 1
fi

rm -rf "$EMBED_DIR"
mkdir -p "$EMBED_DIR/skills" "$EMBED_DIR/templates" "$EMBED_DIR/rules"

cp -R "${SKILL_DIRS[@]}" "$EMBED_DIR/skills/"
cp "$TMP/src/.agents/templates/"*.md "$EMBED_DIR/templates/"
cp "$TMP/src/.agents/rules/"*.md "$EMBED_DIR/rules/"
cp "$TMP/src/VERSION" "$EMBED_DIR/VERSION"

SKILL_COUNT="$(find "$EMBED_DIR/skills" -mindepth 1 -maxdepth 1 -type d | wc -l | tr -d ' ')"
TEMPLATE_COUNT="$(find "$EMBED_DIR/templates" -name '*.md' | wc -l | tr -d ' ')"
RULE_COUNT="$(find "$EMBED_DIR/rules" -name '*.md' | wc -l | tr -d ' ')"
VERSION="$(tr -d ' \t\r\n' < "$EMBED_DIR/VERSION")"

echo "sync-kspec: kspec v$VERSION ($SOURCE@$REF)"
echo "sync-kspec: $SKILL_COUNT skills, $TEMPLATE_COUNT templates, $RULE_COUNT rules -> $EMBED_DIR"
