#!/usr/bin/env bash
# Plans-quota E2E (16 checks). Requires: compose stack up and the server on
# :8080. Uses throwaway users per run — safe to re-run without resetting the
# database, and independent of e2e.sh.
set -u
B=http://localhost:8080
ADMIN=http://localhost:4445
FAILURES=0
check() { if [ "$2" = "$3" ]; then echo "PASS: $1"; else echo "FAIL: $1 (expected=$2 actual=$3)"; FAILURES=$((FAILURES+1)); fi }
contains() { if echo "$3" | grep -q "$2"; then echo "PASS: $1"; else echo "FAIL: $1 (wanted '$2' in: $3)"; FAILURES=$((FAILURES+1)); fi }
curl -fs http://localhost:8080/health >/dev/null || { echo "server not healthy on :8080"; exit 1; }

RUN_ID="$(date +%s)-$RANDOM"
USER_A="quota-a-$RUN_ID@test.local"
USER_B="quota-b-$RUN_ID@test.local"

oauth_token() { # EMAIL -> access_token
  local email="$1" jar lr au sc lc rt nx cc cons fin
  jar=$(mktemp)
  lr=$(curl -s -D - -o /dev/null "$B/auth/login")
  sc=$(echo "$lr" | grep -i '^set-cookie: oauth_state=' | sed 's/.*oauth_state=\([^;]*\).*/\1/')
  au=$(echo "$lr" | grep -i '^location:' | sed 's/^[Ll]ocation: //' | tr -d '\r')
  [ -n "$sc" ] || { echo "no state cookie"; return 1; }
  # Hydra needs its own session cookies for the flow resume (CSRF).
  lc=$(curl -s -c "$jar" -b "$jar" -o /dev/null -w '%{redirect_url}' "$au" | sed 's/.*login_challenge=\([^&]*\).*/\1/')
  rt=$(curl -s -X PUT "$ADMIN/admin/oauth2/auth/requests/login/accept?login_challenge=$lc" \
    -H 'Content-Type: application/json' -d "{\"subject\":\"$email\",\"remember\":false}" | jq -r .redirect_to)
  next=$(curl -s -b "$jar" -c "$jar" -o /dev/null -w '%{redirect_url}' "$rt")
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
    *code=*|*error=*) ;;
  esac
  curl -s -H "Cookie: oauth_state=$sc" "$next" | jq -r .access_token
}

AT=$(oauth_token "$USER_A") || { echo "FAIL: login A"; exit 1; }
[ "$AT" != "null" ] && [ -n "$AT" ] || { echo "FAIL: login A"; exit 1; }
H="Authorization: Bearer $AT"

echo "=== build-up: A (hobby) owns 5 projects and 3 running sandboxes ==="
for i in 1 2 3 4 5; do
  r=$(curl -s -o /dev/null -w '%{http_code}' -X POST -H "$H" -H 'Content-Type: application/json' -d "{\"name\":\"q-proj-$i\"}" "$B/v1/projects")
  check "create owned project #$i -> 201" 201 "$r"
done
body=$(curl -s -w '\n%{http_code}' -X POST -H "$H" -H 'Content-Type: application/json' -d '{"name":"q-proj-6"}' "$B/v1/projects")
code=$(echo "$body" | tail -1); body=$(echo "$body" | head -n -1)
check "6th owned project -> 403" 403 "$code"
msg=$(echo "$body" | jq -r .message); check "403 names the plan and limit" 'quota exceeded: plan "hobby" allows 5 owned projects' "$msg"

PID=$(curl -s -H "$H" "$B/v1/projects?limit=1" | jq -r '.data[0].id')
for i in 1 2 3; do
  r=$(curl -s -o /dev/null -w '%{http_code}' -X POST -H "$H" -H 'Content-Type: application/json' -d "{\"name\":\"q-sbx-$i\"}" "$B/v1/projects/$PID/sandboxes")
  check "create running sandbox #$i -> 201" 201 "$r"
done
body=$(curl -s -w '\n%{http_code}' -X POST -H "$H" -H 'Content-Type: application/json' -d '{"name":"q-sbx-4"}' "$B/v1/projects/$PID/sandboxes")
code=$(echo "$body" | tail -1); body=$(echo "$body" | head -n -1)
check "4th running sandbox -> 403" 403 "$code"
msg=$(echo "$body" | jq -r .message); check "403 names the plan and limit" 'quota exceeded: plan "hobby" allows 3 running sandboxes' "$msg"

echo "=== freeing a slot / restart semantics ==="
SID=$(curl -s -H "$H" "$B/v1/projects/$PID/sandboxes?limit=1" | jq -r '[.data[] | select(.stopped_at==null)][0].id')
check "stop one -> 204 (frees slot)" 204 "$(curl -s -o /dev/null -w '%{http_code}' -X DELETE -H "$H" "$B/v1/sandboxes/$SID")"
check "create after freeing -> 201" 201 "$(curl -s -o /dev/null -w '%{http_code}' -X POST -H "$H" -H 'Content-Type: application/json' -d '{"name":"q-sbx-5"}' "$B/v1/projects/$PID/sandboxes")"
check "restart when at cap -> 403 (quota applies to restart too)" 403 "$(curl -s -o /dev/null -w '%{http_code}' -X POST -H "$H" -H 'Content-Type: application/json' -d "{\"sandbox_id\":\"$SID\"}" "$B/v1/projects/$PID/sandboxes")"
check "still at cap -> new create 403" 403 "$(curl -s -o /dev/null -w '%{http_code}' -X POST -H "$H" -H 'Content-Type: application/json' -d '{"name":"q-sbx-6"}' "$B/v1/projects/$PID/sandboxes")"

echo "=== membership is free (B as member creates with their own quota) ==="
BT=$(oauth_token "$USER_B") || { echo "FAIL: login B"; exit 1; }
[ "$BT" != "null" ] && [ -n "$BT" ] || { echo "FAIL: login B"; exit 1; }
BH="Authorization: Bearer $BT"
check "A adds B to project -> 200" 200 "$(curl -s -o /dev/null -w '%{http_code}' -X POST -H "$H" -H 'Content-Type: application/json' -d "{\"email\":\"$USER_B\"}" "$B/v1/projects/$PID/members")"
check "B (member) create sandbox in A's project -> 201 (B's own quota)" 201 "$(curl -s -o /dev/null -w '%{http_code}' -X POST -H "$BH" -H 'Content-Type: application/json' -d '{"name":"b-sbx-1"}' "$B/v1/projects/$PID/sandboxes")"

echo "----------------------------------------"
echo "QUOTA FAILURES: $FAILURES"
exit "$FAILURES"