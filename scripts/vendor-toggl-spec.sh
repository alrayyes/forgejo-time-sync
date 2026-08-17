#!/bin/sh
# Refreshes testdata/toggl-openapi.json from Toggl's published Swagger 2.0
# spec. The asset URL is content-hashed and changes on every doc rebuild —
# get the current one from https://engineering.toggl.com/docs/track/openapi/
# (the "Toggl API" download link) and pass it here.
#
# Patches in one gap: Toggl's spec declares no top-level `consumes`, so
# Prism's strict content-type check 415s our JSON POST bodies with no
# useful message. Every operation in this spec is JSON in practice; this
# just makes that explicit instead of undeclared.
set -eu

url=${1:?usage: scripts/vendor-toggl-spec.sh <asset-url-from-the-openapi-docs-page>}
out=e2e/testdata/toggl-openapi.json

curl -sL "$url" -o "$out"
jq '.consumes //= ["application/json"]' "$out" >"$out.tmp"
mv "$out.tmp" "$out"

echo "vendored $out from $url"
