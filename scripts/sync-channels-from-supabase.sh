#!/bin/bash
# Sync channels from Supabase to conf/channels.json

set -e

SUPABASE_URL="${SUPABASE_URL}"
SUPABASE_API_KEY="${SUPABASE_API_KEY}"
CHANNELS_FILE="conf/channels.json"

if [ -z "$SUPABASE_URL" ] || [ -z "$SUPABASE_API_KEY" ]; then
    echo "[WARN] Supabase credentials not set, skipping channel sync"
    exit 0
fi

echo "[INFO] Fetching channels from Supabase..."

# Fetch channels from Supabase
RESPONSE=$(curl -s -X GET \
    "${SUPABASE_URL}/rest/v1/channels?order=created_at.asc" \
    -H "apikey: ${SUPABASE_API_KEY}" \
    -H "Authorization: Bearer ${SUPABASE_API_KEY}")

if [ $? -ne 0 ]; then
    echo "[ERROR] Failed to fetch channels from Supabase"
    exit 1
fi

# Check if response is empty array
if [ "$RESPONSE" = "[]" ]; then
    echo "[INFO] No channels found in Supabase"
    # Keep existing channels.json if it exists
    if [ -f "$CHANNELS_FILE" ]; then
        echo "[INFO] Keeping existing channels.json"
    else
        echo "[]" > "$CHANNELS_FILE"
    fi
    exit 0
fi

# Transform Supabase response to channels.json format
echo "$RESPONSE" | jq 'map({
    is_paused: .is_paused,
    username: .username,
    site: .site,
    framerate: .framerate,
    resolution: .resolution,
    pattern: .pattern,
    max_duration: .max_duration,
    max_filesize: .max_filesize,
    created_at: .created_at,
    streamed_at: .streamed_at
})' > "$CHANNELS_FILE"

CHANNEL_COUNT=$(jq '. | length' "$CHANNELS_FILE")
echo "[SUCCESS] Synced $CHANNEL_COUNT channels from Supabase"
