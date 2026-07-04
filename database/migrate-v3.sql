-- =============================================================================
-- Migration v3: Add SeekStreaming poster/preview columns to recordings table
-- =============================================================================
-- Run this in your Supabase SQL editor after pulling the latest code.
-- It is safe to run multiple times (IF NOT EXISTS).
-- =============================================================================

ALTER TABLE recordings
  ADD COLUMN IF NOT EXISTS seekstreaming_poster_url TEXT,
  ADD COLUMN IF NOT EXISTS seekstreaming_preview_url TEXT;
