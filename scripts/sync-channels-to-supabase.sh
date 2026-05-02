#!/bin/bash
# Sync channels from conf/channels.json to Supabase

set -e

SUPABASE_URL="${SUPABASE_URL}"
SUPABASE_API_KEY="${SUPABASE_API_KEY}"
CHANNELS_FILE="conf/channels.json"

if [ -z "$SUPABASE_URL" ] || [ -z "$SUPABASE_API_KEY" ]; then
    echo "[WARN] Supabase credentials not set, skipping channel sync"
    exit 0
fi

if [ ! -f "$CHANNELS_FILE" ]; then
    echo "[WARN] channels.json not found, skipping sync"
    exit 0
fi

echo "[INFO] Syncing channels to Supabase..."

# Read channels and sync each one
jq -c '.[]' "$CHANNELS_FILE" | while read -r channel; do
    USERNAME=$(echo "$channel" | jq -r '.username')
    SITE=$(echo "$channel" | jq -r '.site')
    IS_PAUSED=$(echo "$channel" | jq -r '.is_paused')
    FRAMERATE=$(echo "$channel" | jq -r '.framerate')
    RESOLUTION=$(echo "$channel" | jq -r '.resolution')
    PATTERN=$(echo "$channel" | jq -r '.pattern')
    MAX_DURATION=$(echo "$channel" | jq -r '.max_duration')
    MAX_FILESIZE=$(echo "$channel" | jq -r '.max_filesize')
    CREATED_AT=$(echo "$channel" | jq -r '.created_at')
    STREAMED_AT=$(echo "$channel" | jq -r '.streamed_at // "null"')
    
    # Prepare JSON payload
    PAYLOAD=$(jq -n \
        --arg username "$USERNAME" \
        --arg site "$SITE" \
        --argjson is_paused "$IS_PAUSED" \
        --argjson framerate "$FRAMERATE" \
        --argjson resolution "$RESOLUTION" \
        --arg pattern "$PATTERN" \
        --argjson max_duration "$MAX_DURATION" \
        --argjson max_filesize "$MAX_FILESIZE" \
        --argjson created_at "$CREATED_AT" \
        --arg streamed_at "$STREAMED_AT" \
        '{
            username: $username,
            site: $site,
            is_paused: $is_paused,
            framerate: $framerate,
            resolution: $resolution,
            pattern: $pattern,
            max_duration: $max_duration,
            max_filesize: $max_filesize,
            created_at: $created_at,
            streamed_at: (if $streamed_at == "null" then null else $streamed_at | tonumber end)
        }')
    
    # Upsert to Supabase (insert or update if exists)
    RESPONSE=$(curl -s -X POST \
        "${SUPABASE_URL}/rest/v1/channels" \
        -H "apikey: ${SUPABASE_API_KEY}" \
        -H "Authorization: Bearer ${SUPABASE_API_KEY}" \
        -H "Content-Type: application/json" \
        -H "Prefer: resolution=merge-duplicates" \
        -d "$PAYLOAD")
    
    if [ $? -eq 0 ]; then
        echo "[SUCCESS] Synced channel: $USERNAME"
    else
        echo "[ERROR] Failed to sync channel: $USERNAME"
    fi
done

echo "[INFO] Channel sync completed"
