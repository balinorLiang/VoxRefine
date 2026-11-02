#!/usr/bin/env bash

set -euo pipefail

BASE_URL="${BASE_URL:-http://127.0.0.1:8080}"
COOKIE_JAR="$(mktemp)"
BODY_FILE="$(mktemp)"
HEADER_FILE="$(mktemp)"
trap 'rm -f "$COOKIE_JAR" "$BODY_FILE" "$HEADER_FILE"' EXIT

pass() {
  printf 'PASS  %s\n' "$1"
}

fail() {
  printf 'FAIL  %s\n' "$1" >&2
  if [ -s "$BODY_FILE" ]; then
    printf 'Response body:\n' >&2
    cat "$BODY_FILE" >&2
    printf '\n' >&2
  fi
  exit 1
}

expect_status() {
  local expected="$1"
  local label="$2"
  local status
  shift 2
  if ! status="$(curl --silent --show-error --max-time 15 --output "$BODY_FILE" --write-out '%{http_code}' "$@")"; then
    fail "$label: request failed"
  fi
  if [ "$status" != "$expected" ]; then
    fail "$label: got HTTP $status, want $expected"
  fi
  pass "$label"
}

expect_body() {
  local needle="$1"
  local label="$2"
  grep -Fq -- "$needle" "$BODY_FILE" || fail "$label: response does not contain $needle"
}

expect_regex() {
  local pattern="$1"
  local label="$2"
  grep -Eq -- "$pattern" "$BODY_FILE" || fail "$label: response does not match $pattern"
}

expect_cache_header() {
  local path="$1"
  local label="$2"
  if ! curl --silent --show-error --max-time 15 --dump-header "$HEADER_FILE" --output /dev/null "$BASE_URL$path"; then
    fail "$label: request failed"
  fi
  grep -Eiq '^cache-control:[[:space:]]*no-cache' "$HEADER_FILE" || fail "$label: missing no-cache header"
  pass "$label"
}

expect_status 200 "health endpoint" "$BASE_URL/api/health"
expect_body '"status":"ok"' "health endpoint"
expect_body '"database":"ok"' "health endpoint"

expect_status 200 "v2 home page" "$BASE_URL/"
expect_body '多音字检查器' "v2 home page"
expect_cache_header "/" "v2 home cache header"

expect_status 200 "lite home page" "$BASE_URL/lite/"
expect_body '多音字检查器' "lite home page"
expect_cache_header "/lite/" "lite cache header"

expect_status 200 "term search" "$BASE_URL/api/terms?q=%E9%87%8D%E5%BA%86&limit=1"
expect_body '"word":"重庆"' "term search"

expect_status 200 "text check" -H 'Content-Type: application/json' --data '{"text":"\u6b22\u8fce\u6765\u5230\u91cd\u5e86\uff0c\u94f6\u884c\u884c\u957f\u51fa\u5dee\u4e86\u3002","max_matches":10}' "$BASE_URL/api/check"
expect_body '"engine":"glossary-v2"' "text check"
expect_body '"text":"重庆"' "text check"

expect_status 200 "stats endpoint" "$BASE_URL/api/stats"
expect_regex '"terms":[1-9][0-9]*' "stats endpoint"

expect_status 401 "anonymous protected endpoint" "$BASE_URL/api/my/cases"

email="smoke-$(date +%s)-$$@example.invalid"
password='SmokePass!123'
register_payload="$(printf '{"email":"%s","password":"%s","display_name":"Smoke User"}' "$email" "$password")"
expect_status 201 "register user" -c "$COOKIE_JAR" -H 'Content-Type: application/json' --data "$register_payload" "$BASE_URL/api/auth/register"
expect_body "$(printf '"email":"%s"' "$email")" "register user"

expect_status 200 "authenticated session" -b "$COOKIE_JAR" "$BASE_URL/api/auth/me"
expect_body "$(printf '"email":"%s"' "$email")" "authenticated session"

case_payload='{"term_id":"chongqing","wrong_text":"\u91cd\uff08zh\u00f2ng\uff09\u5e86","correct_pinyin":"ch\u00f3ng q\u00ecng","note":"smoke test"}'
expect_status 201 "create case" -b "$COOKIE_JAR" -H 'Content-Type: application/json' --data "$case_payload" "$BASE_URL/api/cases"
expect_body '"status":"pending"' "create case"

expect_status 200 "list own cases" -b "$COOKIE_JAR" "$BASE_URL/api/my/cases"
expect_body '"wrong_text":"重（zhòng）庆"' "list own cases"

expect_status 200 "logout" -b "$COOKIE_JAR" -c "$COOKIE_JAR" -X POST "$BASE_URL/api/auth/logout"
expect_status 200 "cleared session" -b "$COOKIE_JAR" "$BASE_URL/api/auth/me"
expect_body '"user":null' "cleared session"

login_payload="$(printf '{"email":"%s","password":"%s"}' "$email" "$password")"
expect_status 200 "login user" -b "$COOKIE_JAR" -c "$COOKIE_JAR" -H 'Content-Type: application/json' --data "$login_payload" "$BASE_URL/api/auth/login"
expect_body "$(printf '"email":"%s"' "$email")" "login user"

printf 'Smoke tests passed against %s\n' "$BASE_URL"
