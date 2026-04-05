#!/bin/bash
#
# Run accessibility audit against a live Freeplay instance.
# Usage: ./test/a11y.sh <freeplay-binary>
#
# Starts the server with testdata on a temporary port, runs pa11y
# against all pages, then shuts the server down.

set -e

BINARY="${1:?usage: $0 <freeplay-binary>}"
PORT=18919
BASE="http://localhost:${PORT}"
TMPDATA=$(mktemp -d)
cleanup() { kill $SERVER_PID 2>/dev/null; wait $SERVER_PID 2>/dev/null || true; rm -rf "$TMPDATA"; }
trap cleanup EXIT

# Use a test game that exists in testdata
CONSOLE="NES"
ROM="Bad%20Dudes.zip"

# Build a temporary data directory with our port override
for d in bios cache covers manuals roms saves; do
    ln -s "$(pwd)/testdata/$d" "$TMPDATA/$d"
done
cat > "$TMPDATA/freeplay.toml" <<EOF
port = ${PORT}

[roms.NES]
path = "roms/nes"
core = "fceumm"
EOF

# Start the server in the background
"$BINARY" -data "$TMPDATA" &
SERVER_PID=$!

# Wait for the server to be ready
for i in $(seq 1 30); do
    if curl -sf "${BASE}/api/health" >/dev/null 2>&1; then
        break
    fi
    sleep 0.2
done

echo "Running accessibility audit..."
echo

PAGES=(
    "${BASE}/"
    "${BASE}/details?console=${CONSOLE}&rom=${ROM}"
)

# The play page is audited separately: EmulatorJS renders unlabelled
# form controls inside #game that we cannot fix (vendored), so we
# hide that subtree from the audit.
PLAY_PAGE="${BASE}/play?console=${CONSOLE}&rom=${ROM}"

FAILED=0
for page in "${PAGES[@]}"; do
    echo "Auditing: $page"
    if ! npx --yes pa11y --standard WCAG2AA --wait 1000 "$page"; then
        FAILED=1
    fi
    echo
done

echo "Auditing: $PLAY_PAGE (excluding #game)"
if ! npx --yes pa11y --standard WCAG2AA --wait 1000 --hide-elements "#game" "$PLAY_PAGE"; then
    FAILED=1
fi
echo

if [ "$FAILED" -eq 1 ]; then
    echo "Accessibility issues found."
    exit 1
fi

echo "All pages pass WCAG 2.0 AA."
