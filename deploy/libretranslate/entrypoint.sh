#!/bin/sh
# Custom entrypoint for LibreTranslate that fixes the broken es→en v1.9 model.
#
# The Argos Translate es→en v1.9 model shipped with LibreTranslate v1.8.4
# produces repetition-loop garbage for ALL Spanish input. The v1.0 model
# (originally from the Argos Translate Google Drive) works correctly.
# We host a copy on our GitHub Releases for reliable automated downloads.
#
# This script downloads the working model on first boot and replaces the
# broken one. The fix persists in the libretranslate_data volume.
#
# See: https://github.com/LibreTranslate/LibreTranslate/issues/183
#      https://github.com/LibreTranslate/LibreTranslate/issues/710

PACKAGES="/home/libretranslate/.local/share/argos-translate/packages"
MODEL_URL="https://github.com/WAN-Ninjas/AmityVox/releases/download/v0.0.1-assets/es_en.argosmodel"

fix_es_model() {
    # Skip if already fixed (v1.0 model present).
    if [ -d "$PACKAGES/es_en" ]; then
        return 0
    fi

    # Skip if broken model doesn't exist yet.
    if [ ! -d "$PACKAGES/translate-es_en-1_9" ]; then
        return 0
    fi

    echo "[amityvox] Detected broken es→en v1.9 model, downloading fix..."

    python3 << PYEOF
import urllib.request, zipfile, os, shutil

url = "$MODEL_URL"
packages = "$PACKAGES"
dest = "/tmp/es_en.argosmodel"

try:
    # GitHub Releases provides direct downloads with no anti-bot pages.
    req = urllib.request.Request(url, headers={"User-Agent": "AmityVox/1.0"})
    with urllib.request.urlopen(req) as resp, open(dest, "wb") as f:
        while True:
            chunk = resp.read(65536)
            if not chunk:
                break
            f.write(chunk)

    size = os.path.getsize(dest)
    if size < 1000000:
        raise Exception(f"Download too small ({size} bytes), probably an error page")

    # Verify it's a valid zip and extract.
    with zipfile.ZipFile(dest, "r") as z:
        z.extractall(packages)

    # Remove the broken model.
    broken = os.path.join(packages, "translate-es_en-1_9")
    if os.path.isdir(broken):
        shutil.rmtree(broken)
    os.remove(dest)
    print("[amityvox] Successfully replaced es→en model. Restart to load it.")
except Exception as e:
    print(f"[amityvox] Warning: failed to fix es→en model: {e}")
    if os.path.exists(dest):
        os.remove(dest)
PYEOF
}

# Run the fix in the background so it doesn't block LibreTranslate startup.
(
    # Wait for LibreTranslate to finish downloading models.
    while [ ! -d "$PACKAGES/translate-es_en-1_9" ] && [ ! -d "$PACKAGES/es_en" ]; do
        sleep 5
    done
    fix_es_model
) &

# Hand off to LibreTranslate's original entrypoint.
exec ./scripts/entrypoint.sh "$@"
