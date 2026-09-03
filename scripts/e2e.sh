#!/usr/bin/env bash
# End-to-end test for the sandbox API (34 checks).
# Requires: compose stack up and the server on :8080. Uses throwaway users
# per run, so it is safe to re-run without resetting the database.
# Emulates the browser OAuth flow using Hydra's admin API for login/consent accept.
set -u
curl -fs http://localhost:8080/health >/dev/null || { echo "server not healthy on :8080"; exit 1; }
RUN_ID="$(date +%s)-$RANDOM"
USER_A="e2e-a-$RUN_ID@test.local"
USER_B="e2e-b-$RUN_ID@test.local"
USER_X="nobody-$RUN_ID@test.local"  # never signs up; used for 404 checks
BASE=http://localhost:8080
HYDRA=http://localhost:4444
ADMIN=http://localhost:4445
FAILURES=0

check() { # name expected actual
  if [ "$2" = "$3" ]; then echo "PASS: $1"; else echo "FAIL: $1 (expected=$2 actual=$3)"; FAILURES=$((FAILURES+1)); fi
}
check_contains() {
  if echo "$3" | grep -q "$2"; then echo "PASS: $1"; else echo "FAIL: $1 (wanted '$2' in: $3)"; FAILURES=$((FAILURES+1)); fi
}

# oauth_token EMAIL -> prints access_token, emulating the browser flow
oauth_token() {
  local email="$1"
  local jar login_resp auth_url state_cookie login_challenge redirect next cc consent scopes aud final callback_url
  jar=$(mktemp)
  login_resp=$(curl -s -D - -o /dev/null "$BASE/auth/login")
  state_cookie=$(echo "$login_resp" | grep -i '^set-cookie: oauth_state=' | sed 's/.*oauth_state=\([^;]*\).*/\1/')
  auth_url=$(echo "$login_resp" | grep -i '^location:' | sed 's/^[Ll]ocation: //' | tr -d '\r')
  [ -n "$state_cookie" ] || { echo "no state cookie"; return 1; }
  # Hydra needs its own session cookies for the flow resume (CSRF).
  login_challenge=$(curl -s -c "$jar" -b "$jar" -o /dev/null -w '%{redirect_url}' "$auth_url" | sed 's/.*login_challenge=\([^&]*\).*/\1/')
  redirect=$(curl -s -X PUT "$ADMIN/admin/oauth2/auth/requests/login/accept?login_challenge=$login_challenge" \
    -H 'Content-Type: application/json' -d "{\"subject\":\"$email\",\"remember\":false}" | jq -r .redirect_to)
  next=$(curl -s -b "$jar" -c "$jar" -o /dev/null -w '%{redirect_url}' "$redirect")
  case "$next" in
    *consent_challenge=*)
      cc=${next#*consent_challenge=}; cc=${cc%%&*}
      consent=$(curl -s "$ADMIN/admin/oauth2/auth/requests/consent?consent_challenge=$cc")
      scopes=$(echo "$consent" | jq -c .requested_scope)
      aud=$(echo "$consent" | jq -c .requested_access_token_audience)
      final=$(curl -s -X PUT "$ADMIN/admin/oauth2/auth/requests/consent/accept?consent_challenge=$cc" \
        -H 'Content-Type: application/json' \
        -d "{\"grant_scope\":$scopes,\"grant_access_token_audience\":$aud,\"remember\":false,\"session\":{}}" | jq -r .redirect_to)
      next=$(curl -s -b "$jar" -c "$jar" -o /dev/null -w '%{redirect_url}' "$final")
      ;;
    *code=*|*error=*) ;; # consent skipped or errored — handled below
  esac
  callback_url="$next"
  curl -s -H "Cookie: oauth_state=$state_cookie" "$callback_url"
}

token_of() { oauth_token "$1" | jq -r .access_token; }

echo "=== Phase 1: auth edges ==="
r=$(curl -s -o /dev/null -w '%{http_code}' "$BASE/v1/projects")
check "no token -> 401" 401 "$r"
r=$(curl -s -o /dev/null -w '%{http_code}' -H "Authorization: Bearer garbage" "$BASE/v1/projects")
check "garbage token -> 401" 401 "$r"

echo "=== Phase 2: OAuth flow (user A) ==="
A_TOKEN=$(token_of "$USER_A")
[ "$A_TOKEN" != "null" ] && [ -n "$A_TOKEN" ] && echo "PASS: login A got token" || { echo "FAIL: login A: $(oauth_token "$USER_A")"; exit 1; }
claims=$(echo "$A_TOKEN" | cut -d. -f2 | tr '_-' '/+' | awk '{l=length($0)%4; if(l==2)$0=$0"=="; else if(l==3)$0=$0"="; print}' | base64 -d 2>/dev/null | jq -r '.sub + " " + .iss + " " + .client_id')
check "token sub matches run user" "$USER_A" "$(echo "$claims" | awk '{print $1}')"
check "token sub/iss/client_id" "$USER_A http://localhost:4444 e2b-assignment" "$claims"
exp=$(echo "$A_TOKEN" | cut -d. -f2 | base64 -d 2>/dev/null | jq .exp); now=$(date +%s)
[ "$exp" -gt "$now" ] && echo "PASS: token has future exp" || { echo "FAIL: exp=$exp now=$now"; FAILURES=$((FAILURES+1)); }
AH="Authorization: Bearer $A_TOKEN"

echo "=== Phase 3: project CRUD + contract ==="
r=$(curl -s -o /dev/null -w '%{http_code}' -H "$AH" "$BASE/v1/projects")
check "list projects (empty) -> 200" 200 "$r"
body=$(curl -s -H "$AH" "$BASE/v1/projects")
check_contains "empty data is []" '"data":\[\]' "$body"
r=$(curl -s -o /dev/null -w '%{http_code}' -X POST -H "$AH" -H 'Content-Type: application/json' -d 'not json' "$BASE/v1/projects")
check "invalid json -> 400" 400 "$r"
r=$(curl -s -o /dev/null -w '%{http_code}' -X POST -H "$AH" -H 'Content-Type: application/json' -d '{"name":""}' "$BASE/v1/projects")
check "empty name -> 400" 400 "$r"
r=$(curl -s -o /dev/null -w '%{http_code}' -X POST -H "$AH" -H 'Content-Type: application/json' -d "{\"name\":\"$(printf 'x%.0s' {1..300})\"}" "$BASE/v1/projects")
check "name >255 -> 400" 400 "$r"
P1=$(curl -s -X POST -H "$AH" -H 'Content-Type: application/json' -d '{"name":"project-one"}' "$BASE/v1/projects")
check_contains "create project -> snake_case + id" '"id"' "$P1"
if echo "$P1" | grep -q '"Name"'; then echo "FAIL: PascalCase leak in project DTO"; FAILURES=$((FAILURES+1)); else echo "PASS: no PascalCase leak"; fi
P1_ID=$(echo "$P1" | jq -r .id)
P2_ID=$(curl -s -X POST -H "$AH" -H 'Content-Type: application/json' -d '{"name":"project-two"}' "$BASE/v1/projects" | jq -r .id)
[ -n "$P1_ID" ] && [ -n "$P2_ID" ] && echo "PASS: two projects created" || { echo "FAIL: project ids"; exit 1; }

echo "--- project name rules (mandatory, unique per owner) ---"
r=$(curl -s -o /dev/null -w '%{http_code}' -X POST -H "$AH" -H 'Content-Type: application/json' -d '{}' "$BASE/v1/projects")
check "missing project name -> 400" 400 "$r"
r=$(curl -s -o /dev/null -w '%{http_code}' -X POST -H "$AH" -H 'Content-Type: application/json' -d '{"name":"   "}' "$BASE/v1/projects")
check "whitespace-only project name -> 400" 400 "$r"
r=$(curl -s -o /dev/null -w '%{http_code}' -X POST -H "$AH" -H 'Content-Type: application/json' -d '{"name":"project-one"}' "$BASE/v1/projects")
check "duplicate project name -> 409" 409 "$r"
r=$(curl -s -o /dev/null -w '%{http_code}' -X POST -H "$AH" -H 'Content-Type: application/json' -d '{"name":"PROJECT-ONE"}' "$BASE/v1/projects")
check "duplicate project name case-insensitive -> 409" 409 "$r"
PADP=$(curl -s -X POST -H "$AH" -H 'Content-Type: application/json' -d '{"name":"  padded-proj  "}' "$BASE/v1/projects")
check "project name trimmed on store" "padded-proj" "$(echo "$PADP" | jq -r .name)"
r=$(curl -s -o /dev/null -w '%{http_code}' -X POST -H "$AH" -H 'Content-Type: application/json' -d '{"name":"padded-proj"}' "$BASE/v1/projects")
check "duplicate of trimmed project name -> 409" 409 "$r"
body=$(curl -s -H "$AH" "$BASE/v1/projects/$P1_ID")
check_contains "get project dto" '"name":"project-one"' "$body"
r=$(curl -s -o /dev/null -w '%{http_code}' -H "$AH" "$BASE/v1/projects/00000000-0000-0000-0000-000000000000")
check "unknown project -> 404" 404 "$r"
body=$(curl -s -H "$AH" "$BASE/v1/projects?limit=1&offset=99999999999999")
check_contains "huge offset clamped -> 200 not 500" '"total"' "$body"
r=$(curl -s -o /dev/null -w '%{http_code}' -X POST -H "$AH" -H 'Content-Type: application/json' -d "{\"email\":\"$USER_X\"}" "$BASE/v1/projects/$P1_ID/members")
check "add unknown user -> 404" 404 "$r"

echo "=== Phase 4: membership (user B) ==="
B_TOKEN=$(token_of "$USER_B")
[ "$B_TOKEN" != "null" ] && [ -n "$B_TOKEN" ] && echo "PASS: login B got token" || { echo "FAIL: login B"; exit 1; }
BH="Authorization: Bearer $B_TOKEN"
r=$(curl -s -o /dev/null -w '%{http_code}' -H "$BH" "$BASE/v1/projects/$P1_ID")
check "non-member B get A's project -> 403" 403 "$r"
r=$(curl -s -o /dev/null -w '%{http_code}' -X POST -H "$BH" -H 'Content-Type: application/json' -d '{"name":"x"}' "$BASE/v1/projects/$P1_ID/sandboxes")
check "non-member B create sandbox -> 403" 403 "$r"
r=$(curl -s -o /dev/null -w '%{http_code}' -X POST -H "$AH" -H 'Content-Type: application/json' -d "{\"email\":\"$USER_B\"}" "$BASE/v1/projects/$P1_ID/members")
check "owner A adds B -> 200" 200 "$r"
body=$(curl -s -X POST -H "$AH" -H 'Content-Type: application/json' -d "{\"email\":\"$USER_B\"}" "$BASE/v1/projects/$P1_ID/members")
check_contains "duplicate member -> 409" '"CONFLICT"' "$body"
r=$(curl -s -o /dev/null -w '%{http_code}' -H "$BH" "$BASE/v1/projects/$P1_ID")
check "member B get project -> 200" 200 "$r"
r=$(curl -s -o /dev/null -w '%{http_code}' -X POST -H "$BH" -H 'Content-Type: application/json' -d '{"email":"someone@else.io"}' "$BASE/v1/projects/$P1_ID/members")
check "member B (not owner) add member -> 403" 403 "$r"
r=$(curl -s -o /dev/null -w '%{http_code}' -X POST -H "$BH" -H 'Content-Type: application/json' -d '{"name":"project-one"}' "$BASE/v1/projects")
check "B creates own project with same name -> 201 (per-owner scoping)" 201 "$r"

echo "=== Phase 5: sandbox lifecycle ==="
S1=$(curl -s -X POST -H "$AH" -H 'Content-Type: application/json' -d '{"name":"sbx-1"}' "$BASE/v1/projects/$P1_ID/sandboxes")
check_contains "create sandbox dto stopped_at null" '"stopped_at":null' "$S1"
check_contains "sandbox snake_case fields" '"project_id"' "$S1"
S1_ID=$(echo "$S1" | jq -r .id)

echo "--- sandbox name rules (mandatory, unique per project) ---"
r=$(curl -s -o /dev/null -w '%{http_code}' -X POST -H "$AH" -H 'Content-Type: application/json' -d '{}' "$BASE/v1/projects/$P1_ID/sandboxes")
check "missing name -> 400" 400 "$r"
r=$(curl -s -o /dev/null -w '%{http_code}' -X POST -H "$AH" -H 'Content-Type: application/json' -d '{"name":"   "}' "$BASE/v1/projects/$P1_ID/sandboxes")
check "whitespace-only name -> 400" 400 "$r"
r=$(curl -s -o /dev/null -w '%{http_code}' -X POST -H "$AH" -H 'Content-Type: application/json' -d '{"name":"sbx-1"}' "$BASE/v1/projects/$P1_ID/sandboxes")
check "duplicate name same project -> 409" 409 "$r"
r=$(curl -s -o /dev/null -w '%{http_code}' -X POST -H "$AH" -H 'Content-Type: application/json' -d '{"name":"SBX-1"}' "$BASE/v1/projects/$P1_ID/sandboxes")
check "duplicate name case-insensitive -> 409" 409 "$r"
PAD=$(curl -s -X POST -H "$AH" -H 'Content-Type: application/json' -d '{"name":"  pad  "}' "$BASE/v1/projects/$P1_ID/sandboxes")
check "name trimmed on store" "pad" "$(echo "$PAD" | jq -r .name)"
r=$(curl -s -o /dev/null -w '%{http_code}' -X POST -H "$AH" -H 'Content-Type: application/json' -d '{"name":"pad"}' "$BASE/v1/projects/$P1_ID/sandboxes")
check "duplicate of trimmed name -> 409" 409 "$r"
r=$(curl -s -o /dev/null -w '%{http_code}' -X POST -H "$AH" -H 'Content-Type: application/json' -d '{"name":"sbx-1"}' "$BASE/v1/projects/$P2_ID/sandboxes")
check "same name other project -> 201 (per-project scoping)" 201 "$r"
body=$(curl -s -X POST -H "$AH" -H 'Content-Type: application/json' -d "{\"sandbox_id\":\"$S1_ID\"}" "$BASE/v1/projects/$P1_ID/sandboxes")
check_contains "restart running -> 200 no-op" '"stopped_at":null' "$body"
r=$(curl -s -o /dev/null -w '%{http_code}' -X POST -H "$AH" -H 'Content-Type: application/json' -d "{\"sandbox_id\":\"$S1_ID\"}" "$BASE/v1/projects/$P2_ID/sandboxes")
check "cross-project restart -> 404" 404 "$r"
r=$(curl -s -o /dev/null -w '%{http_code}' -X DELETE -H "$AH" "$BASE/v1/sandboxes/$S1_ID")
check "stop sandbox -> 204" 204 "$r"
r=$(curl -s -o /dev/null -w '%{http_code}' -X DELETE -H "$AH" "$BASE/v1/sandboxes/$S1_ID")
check "stop already stopped -> 409" 409 "$r"
r=$(curl -s -o /dev/null -w '%{http_code}' -X DELETE -H "$AH" "$BASE/v1/sandboxes/00000000-0000-0000-0000-000000000000")
check "stop unknown sandbox -> 404" 404 "$r"
body=$(curl -s -X POST -H "$AH" -H 'Content-Type: application/json' -d "{\"sandbox_id\":\"$S1_ID\"}" "$BASE/v1/projects/$P1_ID/sandboxes")
check_contains "restart stopped -> running" '"stopped_at":null' "$body"
r=$(curl -s -o /dev/null -w '%{http_code}' -X DELETE -H "$BH" "$BASE/v1/sandboxes/$(echo "$S1" | jq -r .id)")
check "member B can stop project sandbox -> 204" 204 "$r"
r=$(curl -s -o /dev/null -w '%{http_code}' -X DELETE -H "$BH" "$BASE/v1/sandboxes/00000000-0000-0000-0000-000000000001")
check "B stop foreign sandbox -> 404 (no leak)" 404 "$r"
r=$(curl -s -o /dev/null -w '%{http_code}' -H "$AH" "$BASE/v1/projects/$P1_ID/sandboxes?limit=abc&offset=-3")
check "bad pagination params -> 200 defaults" 200 "$r"
echo "----------------------------------------"
echo "PHASE-1 FAILURES: $FAILURES"
exit "$FAILURES"