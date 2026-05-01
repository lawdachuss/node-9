# 🚀 How to Start GoOnDVR on AWS EC2

## Quick Start (3 Commands)

Connect to your EC2 instance and run:

```bash
# 1. Go to the application directory
cd /home/ubuntu/goondvr

# 2. Start all containers
sudo docker compose up -d

# 3. Check status
sudo docker compose ps
```

That's it! GoOnDVR should now be running.

---

## 📊 Verify It's Working

### Check Container Status:
```bash
sudo docker compose ps
```

**Expected output:**
```
NAME        STATUS          PORTS
goondvr     Up 2 minutes    0.0.0.0:8080->8080/tcp
byparr-lb   Up 2 minutes    0.0.0.0:8191->80/tcp
byparr_1    Up 2 minutes    
byparr_2    Up 2 minutes
```

All containers should show **"Up"** status.

---

### Check Logs:
```bash
sudo docker logs goondvr --tail 50
```

**Good signs:**
- ✅ "starting to record `username`"
- ✅ "stream type: HLS, resolution 1080p"
- ✅ "duration: 00:01:23, filesize: 45.2 MB"

**Bad signs:**
- ❌ "Cloudflare blocked"
- ❌ "channel is offline" (for all channels)
- ❌ "no stream URL found"

---

### Check Web UI:
Open in your browser:
```
http://54.210.37.19:8080
```

You should see the dashboard with your channels.

---

## 🔄 Common Commands

### Start GoOnDVR:
```bash
cd /home/ubuntu/goondvr
sudo docker compose up -d
```

### Stop GoOnDVR:
```bash
cd /home/ubuntu/goondvr
sudo docker compose stop
```

### Restart GoOnDVR:
```bash
cd /home/ubuntu/goondvr
sudo docker compose restart
```

### View Logs (Real-time):
```bash
sudo docker logs -f goondvr
```
Press `Ctrl+C` to exit

### View Logs (Last 100 lines):
```bash
sudo docker logs goondvr --tail 100
```

### Check Status:
```bash
sudo docker compose ps
```

### Rebuild (if you made code changes):
```bash
cd /home/ubuntu/goondvr
sudo docker compose down
sudo docker compose up -d --build
```

---

## 🆘 Troubleshooting

### Issue: Containers won't start

**Check what's wrong:**
```bash
sudo docker compose logs
```

**Common fixes:**
```bash
# Clean up and restart
sudo docker compose down
sudo docker system prune -f
sudo docker compose up -d
```

---

### Issue: "Cloudflare blocked" errors

**Fix: Update cookies**

1. Get fresh cookies from your browser:
   - Visit https://chaturbate.com
   - Press F12 → Network tab
   - Refresh page
   - Copy Cookie header from any request

2. Update settings:
```bash
nano conf/settings.json
```

3. Replace the `"cookies"` field with your fresh cookies

4. Restart:
```bash
sudo docker restart goondvr
```

---

### Issue: Out of disk space

**Check disk:**
```bash
df -h /
```

**Clean up:**
```bash
cd /home/ubuntu/goondvr
./scripts/cleanup-disk.sh
sudo docker system prune -af
```

---

### Issue: Out of memory

**Check memory:**
```bash
free -h
```

**Restart to free memory:**
```bash
sudo docker compose restart
```

---

## 📋 Complete Startup Checklist

Run these commands in order:

```bash
# 1. Navigate to directory
cd /home/ubuntu/goondvr

# 2. Pull latest code (optional)
git pull origin main

# 3. Stop existing containers
sudo docker compose down

# 4. Start containers
sudo docker compose up -d

# 5. Wait 30 seconds for startup
sleep 30

# 6. Check status
sudo docker compose ps

# 7. Check logs
sudo docker logs goondvr --tail 50

# 8. Check if recording
sudo docker exec goondvr ls -lh /usr/src/app/videos/
```

---

## 🎯 First Time Setup

If this is your first time or you need to reinstall:

```bash
# 1. Clone repository
cd /home/ubuntu
git clone https://github.com/vasud3v/record.git goondvr
cd goondvr

# 2. Create directories
mkdir -p videos/completed database conf

# 3. Copy settings (if you have a backup)
# cp ~/settings-backup.json conf/settings.json
# cp ~/channels-backup.json conf/channels.json

# 4. Start containers
sudo docker compose up -d --build

# 5. Wait for startup
sleep 60

# 6. Check status
sudo docker compose ps
sudo docker logs goondvr --tail 50
```

---

## 🌐 Access Web UI

Once started, access the web interface:

**URL:** http://54.210.37.19:8080

**Features:**
- View all channels and their status
- Add/remove channels
- Pause/resume recording
- View logs
- Check disk usage
- Configure settings

---

## 🔧 Update Cookies (Most Important!)

Cloudflare 2026 requires fresh cookies every 24-48 hours:

```bash
# 1. Edit settings
nano conf/settings.json

# 2. Update the "cookies" field with fresh cookies from browser

# 3. Save: Ctrl+O, Enter, Ctrl+X

# 4. Restart
sudo docker restart goondvr

# 5. Verify
sudo docker logs -f goondvr
```

You should see channels start recording within 1-2 minutes.

---

## 📊 Monitor System

### Real-time monitoring:
```bash
# Watch logs
sudo docker logs -f goondvr

# Watch container stats
sudo docker stats

# Watch disk usage
watch -n 5 df -h /
```

### Check recordings:
```bash
# List active recordings
sudo docker exec goondvr ls -lh /usr/src/app/videos/

# Count recordings
sudo docker exec goondvr ls -1 /usr/src/app/videos/*.ts /usr/src/app/videos/*.mp4 2>/dev/null | wc -l
```

---

## 🎉 Success Indicators

You'll know it's working when:

1. **Containers are running:**
   ```bash
   sudo docker compose ps
   # All show "Up"
   ```

2. **Logs show recording:**
   ```bash
   sudo docker logs goondvr --tail 20
   # Shows: "starting to record", "duration: XX:XX"
   ```

3. **Files are being created:**
   ```bash
   sudo docker exec goondvr ls -lh /usr/src/app/videos/
   # Shows .ts or .mp4 files with increasing sizes
   ```

4. **Web UI shows online channels:**
   - Visit http://54.210.37.19:8080
   - Channels show green "Online" status
   - Duration and filesize are increasing

---

## 🔄 Auto-Start on Reboot

To make GoOnDVR start automatically when EC2 reboots:

```bash
# Create systemd service
sudo nano /etc/systemd/system/goondvr.service
```

Paste this:
```ini
[Unit]
Description=GoOnDVR Docker Compose
Requires=docker.service
After=docker.service

[Service]
Type=oneshot
RemainAfterExit=yes
WorkingDirectory=/home/ubuntu/goondvr
ExecStart=/usr/bin/docker compose up -d
ExecStop=/usr/bin/docker compose down
TimeoutStartSec=0

[Install]
WantedBy=multi-user.target
```

Enable it:
```bash
sudo systemctl enable goondvr
sudo systemctl start goondvr
```

---

## 📚 Quick Reference

| Task | Command |
|------|---------|
| Start | `sudo docker compose up -d` |
| Stop | `sudo docker compose stop` |
| Restart | `sudo docker compose restart` |
| Logs | `sudo docker logs goondvr --tail 100` |
| Status | `sudo docker compose ps` |
| Rebuild | `sudo docker compose up -d --build` |
| Clean | `sudo docker system prune -af` |

---

**Need help?** Check the logs first:
```bash
sudo docker logs goondvr --tail 100
```

Most issues are related to expired cookies or Cloudflare blocks!
