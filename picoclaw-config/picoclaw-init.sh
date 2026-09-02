#!/bin/sh
# Supervisor entrypoint for the PicoClaw gateway.
#
# LLM provider keys are owner-editable from the console: the backend writes them
# to /shared/llm_keys.json as {"primary":{"provider","key"},"fallback":{...}}.
# This script renders config.json from those keys (building a model_list with the
# primary model plus an optional fallback the bot fails over to automatically),
# runs the gateway, and restarts it whenever the keys change — so the owner never
# has to touch the terminal. Output is mirrored to /shared/picoclaw.log for the
# console's WhatsApp status + QR view.
set -u

HOME_DIR="${HOME:-/root}/.picoclaw"
KEYS_FILE=/shared/llm_keys.json
DISABLED_FILE=/shared/bot_disabled
RESET_FILE=/shared/whatsapp_reset
LOG=/shared/picoclaw.log

mkdir -p "$HOME_DIR/workspace/memory" /shared

# Seed the business persona once (don't clobber edits).
[ -f "$HOME_DIR/workspace/AGENT.md" ] || cp /init/AGENT.md "$HOME_DIR/workspace/AGENT.md"

# Map a provider id to its PicoClaw model string and API base.
provider_model() {
  case "$1" in
    claude) echo "anthropic-messages/claude-sonnet-4-6" ;;
    gemini) echo "gemini/gemini-3.1-flash-lite" ;;
    openai) echo "openai/gpt-5.4" ;;
    *)      echo "" ;;
  esac
}
provider_base() {
  case "$1" in
    claude) echo "https://api.anthropic.com" ;;
    gemini) echo "https://generativelanguage.googleapis.com/v1beta" ;;
    openai) echo "https://api.openai.com/v1" ;;
    *)      echo "" ;;
  esac
}

# A single token capturing everything that should trigger a (re)start: the keys
# file's mtime and whether the owner has paused the bot.
keys_mtime() { stat -c %Y "$KEYS_FILE" 2>/dev/null || echo 0; }
# state_token also flips when the owner requests a WhatsApp "remove connection"
# (RESET_FILE appears), so the running gateway is torn down and the loop re-runs
# to delete the pairing store before starting fresh.
state_token() {
  _r=""; [ -f "$RESET_FILE" ] && _r="-reset"
  if [ -f "$DISABLED_FILE" ]; then echo "$(keys_mtime)-off$_r"; else echo "$(keys_mtime)-on$_r"; fi
}

# Read the owner-set providers (file wins; else fall back to the
# ANTHROPIC_API_KEY env var as a Claude-only primary). Cloud providers use a
# key; a "local" provider uses a base URL + model instead.
load_keys() {
  PRIMARY_PROV=""; PRIMARY_KEY=""; PRIMARY_BASE=""; PRIMARY_MODEL=""
  FALLBACK_PROV=""; FALLBACK_KEY=""; FALLBACK_BASE=""; FALLBACK_MODEL=""
  if [ -s "$KEYS_FILE" ]; then
    PRIMARY_PROV=$(jq -r '.primary.provider // empty' "$KEYS_FILE" 2>/dev/null)
    PRIMARY_KEY=$(jq -r '.primary.key // empty' "$KEYS_FILE" 2>/dev/null)
    PRIMARY_BASE=$(jq -r '.primary.base_url // empty' "$KEYS_FILE" 2>/dev/null)
    PRIMARY_MODEL=$(jq -r '.primary.model // empty' "$KEYS_FILE" 2>/dev/null)
    FALLBACK_PROV=$(jq -r '.fallback.provider // empty' "$KEYS_FILE" 2>/dev/null)
    FALLBACK_KEY=$(jq -r '.fallback.key // empty' "$KEYS_FILE" 2>/dev/null)
    FALLBACK_BASE=$(jq -r '.fallback.base_url // empty' "$KEYS_FILE" 2>/dev/null)
    FALLBACK_MODEL=$(jq -r '.fallback.model // empty' "$KEYS_FILE" 2>/dev/null)
  elif [ -n "${ANTHROPIC_API_KEY:-}" ]; then
    PRIMARY_PROV="claude"; PRIMARY_KEY="$ANTHROPIC_API_KEY"
  fi
}

# is_configured PROV KEY BASE — true when the provider is usable.
is_configured() {
  if [ "$1" = "local" ]; then [ -n "$3" ]; else [ -n "$2" ]; fi
}

# Render config.json from the template + loaded providers, building the
# model_list and the primary/fallback wiring.
render_config() {
  MODELS=$(jq -n '[]')
  add_model() {  # prov key base model
    _prov="$1"; _key="$2"; _base="$3"; _modelid="$4"
    if [ "$_prov" = "local" ]; then
      [ -z "$_base" ] && return 0
      [ -z "$_modelid" ] && return 0
      _m="openai/$_modelid"; _b="$_base"; _k="${_key:-local}"
    else
      _m=$(provider_model "$_prov")
      [ -z "$_m" ] && return 0
      _b=$(provider_base "$_prov"); _k="$_key"
    fi
    MODELS=$(echo "$MODELS" | jq \
      --arg n "$_prov" --arg m "$_m" --arg k "$_k" --arg b "$_b" \
      '. += [{model_name:$n, model:$m, api_keys:[$k], api_base:$b}]')
  }

  add_model "$PRIMARY_PROV" "$PRIMARY_KEY" "$PRIMARY_BASE" "$PRIMARY_MODEL"
  FALLBACKS=$(jq -n '[]')
  if [ -n "$FALLBACK_PROV" ] && is_configured "$FALLBACK_PROV" "$FALLBACK_KEY" "$FALLBACK_BASE"; then
    add_model "$FALLBACK_PROV" "$FALLBACK_KEY" "$FALLBACK_BASE" "$FALLBACK_MODEL"
    FALLBACKS=$(jq -n --arg f "$FALLBACK_PROV" '[$f]')
  fi

  jq --argjson models "$MODELS" --arg primary "$PRIMARY_PROV" --argjson fallbacks "$FALLBACKS" \
    '.model_list = $models
     | .agents.defaults.model_name = $primary
     | .agents.defaults.model_fallbacks = $fallbacks' \
    /init/config.template.json > "$HOME_DIR/config.json"
}

while true; do
  load_keys

  # Owner asked to remove the WhatsApp connection: the gateway is already stopped
  # (RESET_FILE flipped state_token, breaking the watch loop), so it's safe to
  # delete the whatsmeow pairing store. Next start prints a fresh QR.
  if [ -f "$RESET_FILE" ]; then
    echo "Removing WhatsApp connection (owner requested) — deleting pairing store…"
    rm -f "$HOME_DIR"/whatsapp/store.db "$HOME_DIR"/whatsapp/store.db-wal "$HOME_DIR"/whatsapp/store.db-shm
    rm -f "$RESET_FILE"
  fi

  if ! is_configured "$PRIMARY_PROV" "$PRIMARY_KEY" "$PRIMARY_BASE"; then
    echo "Waiting for an AI provider — set it in the Owner console (AI Providers)…"
    sleep 3
    continue
  fi
  if [ -f "$DISABLED_FILE" ]; then
    echo "Bot is disabled by the owner — paused (WhatsApp pairing kept)."
    sleep 3
    continue
  fi

  render_config
  rm -f "$HOME_DIR/.picoclaw.pid"
  : > "$LOG"

  if [ -n "$FALLBACK_PROV" ]; then
    echo "Starting PicoClaw gateway (primary: $PRIMARY_PROV, fallback: $FALLBACK_PROV)…" | tee -a "$LOG"
  else
    echo "Starting PicoClaw gateway (provider: $PRIMARY_PROV)…" | tee -a "$LOG"
  fi
  picoclaw gateway 2>&1 | tee -a "$LOG" &
  PIPE_PID=$!
  LAST_STATE="$(state_token)"

  # Watch: if the keys change or the owner disables the bot, stop the gateway so
  # the loop re-evaluates (restart with new keys, or pause).
  while kill -0 "$PIPE_PID" 2>/dev/null; do
    sleep 3
    if [ "$(state_token)" != "$LAST_STATE" ]; then
      echo "Bot setting changed — applying…"
      [ -f "$HOME_DIR/.picoclaw.pid" ] && kill "$(cat "$HOME_DIR/.picoclaw.pid")" 2>/dev/null
      pkill -x picoclaw 2>/dev/null
      break
    fi
  done

  wait "$PIPE_PID" 2>/dev/null
  sleep 1
done
