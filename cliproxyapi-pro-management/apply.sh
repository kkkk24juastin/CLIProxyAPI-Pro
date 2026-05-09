#!/usr/bin/env bash
set -euo pipefail

CUSTOMIZATION_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
TARGET_DIR="${1:-.}"
TARGET_DIR="$(cd "$TARGET_DIR" && pwd)"

python3 "$CUSTOMIZATION_DIR/apply_customizations.py" "$TARGET_DIR"
