#!/bin/sh
# Refreshes testdata/focus-openapi.json from Toggl's published Swagger 2.0
# spec for the 2.0 (Focus) API. The asset URL is content-hashed and changes
# on every doc rebuild — get the current one from
# https://engineering.toggl.com/docs/focus/openapi/ and pass it here.
#
# Patches in three gaps between the vendored spec and what Prism needs to
# mock it usefully, none of which reflect real API behavior:
#
#   - No top-level `consumes`: every operation in this spec is JSON in
#     practice, but leaving it undeclared makes Prism's strict content-type
#     check 415 our JSON POST bodies with no useful message.
#   - Every operation's `security` requirement demands a bearerAuth token
#     *and* a cookieAuth cookie together, which would 401 a client that
#     only ever sends a bearer token — not how the real API works.
#   - No `minimum` on `id` properties, so Prism's dynamic response
#     generator will happily hand out negative ids. This tool's state
#     cache reads a non-positive id the same as "nothing resolved yet".
set -eu

url=${1:?usage: scripts/vendor-focus-spec.sh <asset-url-from-the-openapi-docs-page>}
out=e2e/testdata/focus-openapi.json

curl -sL "$url" -o "$out"
jq '
  .consumes //= ["application/json"]
  | walk(if type == "object" then del(.security) else . end)
  | walk(
      if type == "object" and has("id") and (.id | type) == "object"
        and .id.type == "integer" and ((.id | has("minimum")) | not)
      then .id += {minimum: 1}
      else .
      end
    )
' "$out" >"$out.tmp"
mv "$out.tmp" "$out"

echo "vendored $out from $url"
