#!/usr/bin/env bash
#
# generate-agent-config.sh - Generate a per-agent config.yaml file.
#
# Usage:
#   ./generate-agent-config.sh --nick <name> --password <pass> [options]

set -euo pipefail

NICK=""
PASSWORD=""
SERVER="localhost:6697"
CHANNELS="#agents-general"
OUTPUT=""

usage() {
    cat <<'EOF'
Usage: generate-agent-config.sh [options]

Generate a per-agent config.yaml file for the Agent Chat system.

Options:
  --nick NAME          Agent nickname (required)
  --password PASS      Agent password (required)
  --server HOST:PORT   IRC server address (default: localhost:6697)
  --channels CHANS     Comma-separated channels (default: #agents-general)
  --output FILE        Output file (default: stdout)
  --help               Show this help message

Examples:
  # Print config to stdout
  ./generate-agent-config.sh --nick agent-alice-1 --password s3cret

  # Write config to a file
  ./generate-agent-config.sh --nick agent-bob-1 --password s3cret \
    --server irc.example.com:6697 \
    --channels "#agents-general,#project-webapp" \
    --output agent-bob-1.yaml
EOF
}

while [[ $# -gt 0 ]]; do
    case "$1" in
        --nick)     NICK="$2"; shift 2 ;;
        --password) PASSWORD="$2"; shift 2 ;;
        --server)   SERVER="$2"; shift 2 ;;
        --channels) CHANNELS="$2"; shift 2 ;;
        --output)   OUTPUT="$2"; shift 2 ;;
        --help)     usage; exit 0 ;;
        *)          echo "error: unknown option: $1" >&2; usage >&2; exit 1 ;;
    esac
done

if [[ -z "$NICK" ]]; then
    echo "error: --nick is required" >&2
    exit 1
fi

if [[ -z "$PASSWORD" ]]; then
    echo "error: --password is required" >&2
    exit 1
fi

# Build YAML channel list from comma-separated input.
CHANNELS_YAML=""
IFS=',' read -ra CHAN_ARRAY <<< "$CHANNELS"
for ch in "${CHAN_ARRAY[@]}"; do
    ch="$(echo "$ch" | xargs)"  # trim whitespace
    CHANNELS_YAML="${CHANNELS_YAML}
    - \"${ch}\""
done

CONFIG="irc:
  server: \"${SERVER}\"
  nick: \"${NICK}\"
  username: \"${NICK}\"
  password: \"${PASSWORD}\"
  realname: \"${NICK}\"
  tls: true
  tls_insecure_skip_verify: false
  sasl: true
  sasl_mech: \"PLAIN\"
  channels:${CHANNELS_YAML}
  reconnect: true
  max_reconnect_attempts: 3
  ping_frequency: \"2m\"
  timeout: \"60s\"
  quit_message: \"agent shutting down\"
  debug: false

log_level: \"info\"
log_format: \"text\"
"

if [[ -n "$OUTPUT" ]]; then
    echo "$CONFIG" > "$OUTPUT"
    echo "Generated config: $OUTPUT"
else
    echo "$CONFIG"
fi
