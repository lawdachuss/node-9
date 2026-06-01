-- Upload Journal: tracks per-host upload state for every video file.
--
-- On crash recovery, the app queries this table to determine which hosts
-- already received a given file, avoiding redundant uploads.
--
-- Run this in the Supabase SQL editor (or apply via migration tool).

CREATE TABLE IF NOT EXISTS upload_journal (
  id            UUID DEFAULT gen_random_uuid() PRIMARY KEY,
  file_hash     TEXT NOT NULL,          -- SHA-256 partial hash (first+last 64KB + size)
  filename      TEXT NOT NULL,          -- original filename for human reference
  host          TEXT NOT NULL,          -- e.g. "GoFile", "CatBox", "Streamtape"
  status        TEXT NOT NULL DEFAULT 'pending',  -- pending | uploading | success | failed
  error_msg     TEXT,
  file_size     BIGINT,
  instance_id   TEXT,
  created_at    TIMESTAMPTZ DEFAULT now(),
  updated_at    TIMESTAMPTZ DEFAULT now(),
  UNIQUE(file_hash, host)
);

CREATE INDEX IF NOT EXISTS idx_upload_journal_hash ON upload_journal(file_hash);
CREATE INDEX IF NOT EXISTS idx_upload_journal_status ON upload_journal(status);
