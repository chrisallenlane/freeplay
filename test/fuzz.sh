#!/bin/bash
#
# Run fuzz tests for freeplay
# Usage: ./test/fuzz.sh [duration]
#
# Note: Go's fuzzer will fail immediately if it finds a known failing input
# in the corpus (testdata/fuzz/*). This is by design - it ensures you fix
# known bugs before searching for new ones. To see failing inputs:
#   ls internal/*/testdata/fuzz/*/
#

set -e

DURATION="${1:-15s}"

# Define fuzz tests: "TestName:Package:Description"
TESTS=(
    "FuzzCacheGet:./internal/details:cache Get with arbitrary input"
    "FuzzCacheGetMalformedJSON:./internal/details:cache Get with malformed JSON on disk"
    "FuzzLoad:./internal/config:TOML config parsing"
    "FuzzCleanName:./internal/igdb:ROM filename cleaning"
    "FuzzNameVariants:./internal/igdb:ROM name variant generation"
    "FuzzTransformImageURL:./internal/igdb:IGDB image URL transformation"
    "FuzzNewFetcher:./internal/igdb:IGDB fetcher construction"
    "FuzzServeSecureFile:./internal/server:path traversal defense"
    "FuzzSafeName:./internal/server:filename sanitization"
)

echo "Running fuzz tests ($DURATION each)..."
echo

for i in "${!TESTS[@]}"; do
    IFS=':' read -r test_name package description <<< "${TESTS[$i]}"
    echo "$((i+1)). Testing $description..."
    go test -fuzz="^${test_name}$" -fuzztime="$DURATION" "$package"
    echo
done

echo "All fuzz tests passed!"
