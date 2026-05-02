# Channel Persistence with Supabase

## Overview

Channels are now automatically synced to/from Supabase, allowing them to persist across GitHub Actions workflow runs.

## How It Works

### 1. On Workflow Start:
- Fetches channels from Supabase database
- If Supabase is empty, uses `CHANNELS_JSON` secret as fallback
- Syncs any new channels from secret to Supabase

### 2. During Recording:
- Channels can be added/modified via Web UI
- Changes are stored in `conf/channels.json` inside the container

### 3. On Workflow End:
- Extracts `channels.json` from the container
- Syncs all channels back to Supabase
- Channels are preserved for the next run

## Setup

### 1. Run the Migration

Execute this SQL in your Supabase SQL Editor:

```sql
-- Create channels table
CREATE TABLE IF NOT EXISTS channels (
    id SERIAL PRIMARY KEY,
    username TEXT NOT NULL UNIQUE,
    site TEXT NOT NULL DEFAULT 'chaturbate',
    is_paused BOOLEAN NOT NULL DEFAULT false,
    framerate INTEGER NOT NULL DEFAULT 30,
    resolution INTEGER NOT NULL DEFAULT 1080,
    pattern TEXT NOT NULL,
    max_duration INTEGER NOT NULL DEFAULT 45,
    max_filesize INTEGER NOT NULL DEFAULT 0,
    created_at BIGINT NOT NULL,
    streamed_at BIGINT,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

CREATE INDEX idx_channels_username ON channels(username);
CREATE INDEX idx_channels_site ON channels(site);

ALTER TABLE channels ENABLE ROW LEVEL SECURITY;

CREATE POLICY "Allow all operations on channels" ON channels
    FOR ALL USING (true) WITH CHECK (true);
```

### 2. Initial Sync

Add your existing channels to Supabase:

```bash
# Set environment variables
export SUPABASE_URL="https://your-project.supabase.co"
export SUPABASE_API_KEY="your-anon-key"

# Run sync script
chmod +x scripts/sync-channels-to-supabase.sh
./scripts/sync-channels-to-supabase.sh
```

### 3. Verify

Check your Supabase dashboard:
- Go to Table Editor
- Open `channels` table
- You should see all your channels

## Usage

### Adding Channels

**Option 1: Via Web UI (Recommended)**
1. Open your dashboard URL
2. Click "Add Channel"
3. Fill in details
4. Channel is automatically synced to Supabase on workflow end

**Option 2: Via Supabase Dashboard**
1. Go to Supabase Table Editor
2. Open `channels` table
3. Click "Insert row"
4. Fill in channel details
5. Next workflow run will load it

**Option 3: Via Local File (Legacy)**
1. Edit `conf/channels.json`
2. Update `CHANNELS_JSON` secret on GitHub
3. Run workflow

### Modifying Channels

- Changes via Web UI are automatically synced
- Changes in Supabase are loaded on next run
- Both methods work seamlessly

### Deleting Channels

- Delete via Web UI (synced on workflow end)
- Or delete from Supabase `channels` table directly

## Benefits

✅ **Persistent** - Channels survive workflow restarts
✅ **Centralized** - Single source of truth in Supabase
✅ **Flexible** - Add channels via UI or database
✅ **Automatic** - No manual secret updates needed
✅ **Backup** - Channels stored safely in database

## Troubleshooting

### Channels not syncing?

Check workflow logs for:
```
[INFO] Syncing channels to Supabase
[SUCCESS] Synced channel: username
```

### Channels not loading?

1. Verify Supabase secrets are set:
   - `SUPABASE_URL`
   - `SUPABASE_API_KEY`

2. Check table exists:
   ```sql
   SELECT * FROM channels;
   ```

3. Check RLS policies allow access

### Want to reset?

Delete all channels from Supabase:
```sql
DELETE FROM channels;
```

Then sync from your local file:
```bash
./scripts/sync-channels-to-supabase.sh
```

## Migration Path

If you're currently using `CHANNELS_JSON` secret:

1. Run the SQL migration (creates table)
2. Workflow will automatically sync secret to Supabase on first run
3. Future runs will use Supabase
4. You can keep the secret as backup

---

**Last Updated:** 2026-05-03
