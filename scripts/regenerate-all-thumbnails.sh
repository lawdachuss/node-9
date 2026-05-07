#!/bin/bash
# Regenerate thumbnails for all videos in Supabase
# This script will:
# 1. Fetch all videos from Supabase
# 2. For each video without a thumbnail, generate one from the video file
# 3. Upload to Pixhost.to
# 4. Update Supabase with the thumbnail URL

set -e

echo "╔════════════════════════════════════════════════════════════╗"
echo "║        📸 REGENERATE ALL THUMBNAILS                       ║"
echo "╚════════════════════════════════════════════════════════════╝"
echo ""

# Check if required tools are installed
if ! command -v ffmpeg &> /dev/null; then
    echo "❌ ffmpeg is not installed"
    echo "Install it with: sudo apt install ffmpeg"
    exit 1
fi

if ! command -v curl &> /dev/null; then
    echo "❌ curl is not installed"
    exit 1
fi

if ! command -v jq &> /dev/null; then
    echo "❌ jq is not installed"
    echo "Install it with: sudo apt install jq"
    exit 1
fi

# Load environment variables
if [ -f .env ]; then
    export $(grep -v '^#' .env | xargs)
fi

if [ -z "$SUPABASE_URL" ] || [ -z "$SUPABASE_API_KEY" ]; then
    echo "❌ SUPABASE_URL and SUPABASE_API_KEY must be set"
    exit 1
fi

echo "📡 Fetching videos from Supabase..."
VIDEOS=$(curl -s "$SUPABASE_URL/rest/v1/video_uploads?select=*" \
    -H "apikey: $SUPABASE_API_KEY" \
    -H "Content-Type: application/json")

TOTAL=$(echo "$VIDEOS" | jq 'length')
echo "✅ Found $TOTAL videos"
echo ""

# Count videos without thumbnails
NO_THUMB=$(echo "$VIDEOS" | jq '[.[] | select(.thumbnail_link == null or .thumbnail_link == "")] | length')
echo "⚠️  $NO_THUMB videos without thumbnails"
echo ""

if [ "$NO_THUMB" -eq 0 ]; then
    echo "✅ All videos already have thumbnails!"
    exit 0
fi

echo "This script needs access to the original video files to generate thumbnails."
echo "Video files should be in the 'videos/completed' directory."
echo ""
read -p "Continue? (y/n) " -n 1 -r
echo ""

if [[ ! $REPLY =~ ^[Yy]$ ]]; then
    echo "Cancelled"
    exit 0
fi

echo ""
echo "Starting thumbnail generation..."
echo ""

GENERATED=0
FAILED=0

# Process each video without a thumbnail
echo "$VIDEOS" | jq -c '.[] | select(.thumbnail_link == null or .thumbnail_link == "")' | while read -r video; do
    ID=$(echo "$video" | jq -r '.id')
    FILENAME=$(echo "$video" | jq -r '.filename')
    STREAMER=$(echo "$video" | jq -r '.streamer_name')
    
    echo "[$((GENERATED + FAILED + 1))/$NO_THUMB] Processing: $FILENAME"
    
    # Try to find the video file
    VIDEO_PATH=""
    if [ -f "videos/completed/$FILENAME" ]; then
        VIDEO_PATH="videos/completed/$FILENAME"
    elif [ -f "videos/$FILENAME" ]; then
        VIDEO_PATH="videos/$FILENAME"
    else
        echo "  ❌ Video file not found, skipping"
        FAILED=$((FAILED + 1))
        continue
    fi
    
    # Generate thumbnail
    THUMB_PATH="/tmp/${FILENAME%.*}_thumb.jpg"
    rm -f "$THUMB_PATH"
    
    echo "  📸 Generating thumbnail..."
    if ffmpeg -y -ss 2 -i "$VIDEO_PATH" -vframes 1 -vf "scale=640:-2" -q:v 2 "$THUMB_PATH" > /dev/null 2>&1; then
        echo "  ✅ Thumbnail generated"
    else
        # Try without seeking for short videos
        if ffmpeg -y -i "$VIDEO_PATH" -vframes 1 -vf "scale=640:-2" -q:v 2 "$THUMB_PATH" > /dev/null 2>&1; then
            echo "  ✅ Thumbnail generated (no seek)"
        else
            echo "  ❌ Failed to generate thumbnail"
            FAILED=$((FAILED + 1))
            continue
        fi
    fi
    
    # Upload to Pixhost.to
    echo "  ☁️  Uploading to Pixhost.to..."
    UPLOAD_RESPONSE=$(curl -s -X POST "https://api.pixhost.to/images" \
        -F "img=@$THUMB_PATH" \
        -F "content_type=1" \
        -F "max_th_size=420" \
        -H "Accept: application/json")
    
    THUMB_URL=$(echo "$UPLOAD_RESPONSE" | jq -r '.show_url // empty')
    
    if [ -z "$THUMB_URL" ]; then
        echo "  ❌ Failed to upload thumbnail"
        echo "  Response: $UPLOAD_RESPONSE"
        FAILED=$((FAILED + 1))
        rm -f "$THUMB_PATH"
        continue
    fi
    
    echo "  ✅ Uploaded: $THUMB_URL"
    
    # Update Supabase
    echo "  💾 Updating Supabase..."
    UPDATE_RESPONSE=$(curl -s -X PATCH "$SUPABASE_URL/rest/v1/video_uploads?id=eq.$ID" \
        -H "apikey: $SUPABASE_API_KEY" \
        -H "Content-Type: application/json" \
        -H "Prefer: return=minimal" \
        -d "{\"thumbnail_link\": \"$THUMB_URL\"}")
    
    echo "  ✅ Database updated"
    GENERATED=$((GENERATED + 1))
    
    # Clean up
    rm -f "$THUMB_PATH"
    echo ""
done

echo "════════════════════════════════════════════════════════════"
echo "COMPLETE"
echo "════════════════════════════════════════════════════════════"
echo ""
echo "✅ Generated: $GENERATED"
echo "❌ Failed: $FAILED"
echo ""
