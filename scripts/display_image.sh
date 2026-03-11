#!/usr/bin/env bash
set -euo pipefail

[[ -n "${DEBUG:-}" ]] && set -x

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
readonly SCRIPT_DIR
# If installed, PROJECT_ROOT is one level up from SCRIPT_DIR (which is /opt/led-service/scripts)
# In development, it is also one level up.
readonly PROJECT_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
readonly RPI_MATRIX_LED_OPTS=(--led-rows=32 --led-cols=64 --led-brightness=50)

# Allow overriding binary and asset paths via environment variables
readonly DEMO_BIN="${DEMO_BIN:-$SCRIPT_DIR/demo}"
readonly IMAGE_VIEWER_BIN="${IMAGE_VIEWER_BIN:-$SCRIPT_DIR/led-image-viewer}"
readonly JINGLE_VOLUME="${JINGLE_VOLUME:-30%}"
readonly JINGLE_SOUND="${JINGLE_SOUND:-$PROJECT_ROOT/assets/splanews.wav}"
readonly JINGLE_IMAGE="${JINGLE_IMAGE:-$PROJECT_ROOT/assets/butacowalk2.gif}"

# OS detection
IS_DARWIN=$([[ "$(uname -s)" == "Darwin" ]] && echo true || echo false)
readonly IS_DARWIN

function log_info() {
    echo "[display-script][INFO] $*"
}

function log_warn() {
    echo "[display-script][WARN] $*" >&2
}

function log_error() {
    echo "[display-script][ERROR] $*" >&2
}

function die() {
    log_error "$1"
    exit 1
}

function validate_args() {
    local image_file="${1:-}"
    local wait_time="${2:-}"

    if [[ -z "$image_file" || -z "$wait_time" ]]; then
        die "Usage: $0 <image_file> <wait_time>"
    fi

    if ! [[ "$wait_time" =~ ^[0-9]+(\.[0-9]+)?$ ]]; then
        die "wait time must be a number: $wait_time"
    fi

    if [[ ! -f "$image_file" ]]; then
        die "input file not found: $image_file"
    fi
}

function select_command() {
    local file_type=$1

    if [[ "$file_type" =~ ^Netpbm\ image\ data ]]; then
        echo "$DEMO_BIN"
    else
        echo "$IMAGE_VIEWER_BIN"
    fi
}

function handle_timeout_exit() {
    local exit_code=$?
    # timeout exits with 124 when the command is stopped by timeout itself.
    [[ $exit_code -eq 124 ]] && return 0
    return $exit_code
}

function display_image_macos() {
    local image_file=$1
    local wait_time=$2
    local base_name="${image_file##*/}"
    local output_file="/tmp/led-display-output-${base_name}"

    log_info "[macOS] Saving image to $output_file instead of hardware display"
    cp "$image_file" "$output_file"
    sleep "$wait_time"
}

function display_image_linux() {
    local image_file=$1
    local wait_time=$2
    local file_type
    local display_cmd
    local -a cmd_args

    file_type=$(file -b "$image_file")
    log_info "detected file type: $file_type"

    display_cmd=$(select_command "$file_type")
    if [[ "$display_cmd" == "$DEMO_BIN" ]]; then
        cmd_args=("${RPI_MATRIX_LED_OPTS[@]}" -D 1)
    else
        cmd_args=(-C "${RPI_MATRIX_LED_OPTS[@]}")
    fi

    timeout "$wait_time" "$display_cmd" "${cmd_args[@]}" "$image_file" || handle_timeout_exit
}

function display_image() {
    local image_file=$1
    local wait_time=$2

    if [[ "$IS_DARWIN" == "true" ]]; then
        display_image_macos "$image_file" "$wait_time"
    else
        display_image_linux "$image_file" "$wait_time"
    fi
}

function play_jingle() {
    log_info "playing jingle"

    if [[ "$IS_DARWIN" == "true" ]]; then
        log_info "[macOS] Audio playback skipped"
        if [[ -f "$JINGLE_IMAGE" ]]; then
            display_image "$JINGLE_IMAGE" 1
        fi
        return 0
    fi

    # Best-effort volume setup.
    if command -v amixer &>/dev/null; then
        amixer -q cset name='PCM Playback Volume' "$JINGLE_VOLUME" || true
    fi

    # Show jingle image and sound playback concurrently.
    if [[ -f "$JINGLE_SOUND" ]] && command -v aplay &>/dev/null; then
        aplay -q "$JINGLE_SOUND" &
    fi

    if [[ -f "$JINGLE_IMAGE" ]]; then
        display_image "$JINGLE_IMAGE" 5
    fi

    # Wait for all background processes (like aplay) to finish.
    wait
}

# Validate inputs before any side effects.
IMAGE_FILE="${1:-}"
WAIT_TIME="${2:-}"
validate_args "$IMAGE_FILE" "$WAIT_TIME"

play_jingle

log_info "image: $IMAGE_FILE"
log_info "wait: $WAIT_TIME"

display_image "$IMAGE_FILE" "$WAIT_TIME"
