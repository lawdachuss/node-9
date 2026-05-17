-- Add all missing columns to video_uploads and channels tables
-- Run this in your Supabase SQL Editor after the previous migrations

-- ============================================================================
-- Add missing metadata columns to video_uploads
-- ============================================================================

ALTER TABLE video_uploads ADD COLUMN IF NOT EXISTS sprite_link TEXT;
ALTER TABLE video_uploads ADD COLUMN IF NOT EXISTS embed_url TEXT;
ALTER TABLE video_uploads ADD COLUMN IF NOT EXISTS filesize BIGINT;
ALTER TABLE video_uploads ADD COLUMN IF NOT EXISTS room_title TEXT;
ALTER TABLE video_uploads ADD COLUMN IF NOT EXISTS tags JSONB DEFAULT '[]';
ALTER TABLE video_uploads ADD COLUMN IF NOT EXISTS viewers INTEGER DEFAULT 0;
ALTER TABLE video_uploads ADD COLUMN IF NOT EXISTS resolution TEXT;
ALTER TABLE video_uploads ADD COLUMN IF NOT EXISTS framerate INTEGER DEFAULT 0;
ALTER TABLE video_uploads ADD COLUMN IF NOT EXISTS recorded_at TIMESTAMPTZ;
ALTER TABLE video_uploads ADD COLUMN IF NOT EXISTS byse_link TEXT;
ALTER TABLE video_uploads ADD COLUMN IF NOT EXISTS sendcm_link TEXT;

-- Create index on recorded_at for efficient queries
CREATE INDEX IF NOT EXISTS idx_video_uploads_recorded_at ON video_uploads(recorded_at DESC);

-- Add comments
COMMENT ON COLUMN video_uploads.sprite_link IS 'URL to preview sprite image';
COMMENT ON COLUMN video_uploads.embed_url IS 'URL for embedded video player';
COMMENT ON COLUMN video_uploads.filesize IS 'File size in bytes';
COMMENT ON COLUMN video_uploads.room_title IS 'Room title from the stream';
COMMENT ON COLUMN video_uploads.tags IS 'Stream tags as JSON array';
COMMENT ON COLUMN video_uploads.viewers IS 'Viewer count at time of recording';
COMMENT ON COLUMN video_uploads.resolution IS 'Recording resolution (e.g. 1920x1080)';
COMMENT ON COLUMN video_uploads.framerate IS 'Recording framerate (e.g. 30)';
COMMENT ON COLUMN video_uploads.recorded_at IS 'Actual recording timestamp';
COMMENT ON COLUMN video_uploads.byse_link IS 'Byse.sx download link';
COMMENT ON COLUMN video_uploads.sendcm_link IS 'SendCM download link';

-- ============================================================================
-- Add compress column to channels
-- ============================================================================

ALTER TABLE channels ADD COLUMN IF NOT EXISTS compress BOOLEAN NOT NULL DEFAULT false;

COMMENT ON COLUMN channels.compress IS 'Whether to compress recordings with ffmpeg';

-- ============================================================================
-- VERIFICATION QUERIES
-- ============================================================================

-- Run these to verify columns were added:
-- SELECT column_name, data_type FROM information_schema.columns WHERE table_name = 'video_uploads' ORDER BY ordinal_position;
-- SELECT column_name, data_type FROM information_schema.columns WHERE table_name = 'channels' ORDER BY ordinal_position;
