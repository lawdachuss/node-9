#!/bin/bash
# AWS EC2 Recording Diagnostics
# Run this ON the EC2 instance to diagnose recording issues

echo "🔍 GoOnDVR AWS Recording Diagnostics"
echo "====================================="
echo ""

cd /home/ubuntu/goondvr || {
    echo "❌ ERROR: /home/ubuntu/goondvr directory not found!"
    echo "   Are you running this on the EC2 instance?"
    exit 1
}

ISSUES=0
WARNINGS=0

# Check 1: Docker
echo "[1/10] Checking Docker..."
if command -v docker &> /dev/null; then
    DOCKER_VERSION=$(docker --version)
    echo "  ✅ Docker installed: $DOCKER_VERSION"
else
    echo "  ❌ Docker not installed"
    ((ISSUES++))
fi

# Check 2: Docker Compose
echo "[2/10] Checking Docker Compose..."
if docker compose version &> /dev/null; then
    COMPOSE_VERSION=$(docker compose version)
    echo "  ✅ Docker Compose installed: $COMPOSE_VERSION"
else
    echo "  ❌ Docker Compose not installed"
    ((ISSUES++))
fi

# Check 3: Container Status
echo "[3/10] Checking containers..."
CONTAINER_STATUS=$(sudo docker compose ps --format json 2>/dev/null)
if [ -n "$CONTAINER_STATUS" ]; then
    echo "  ✅ Containers found"
    
    # Check each container
    RECORDER_STATUS=$(sudo docker inspect goondvr --format='{{.State.Status}}' 2>/dev/null || echo "not found")
    if [ "$RECORDER_STATUS" = "running" ]; then
        echo "     ✅ goondvr: running"
    else
        echo "     ❌ goondvr: $RECORDER_STATUS"
        ((ISSUES++))
    fi
    
    BYPARR_COUNT=$(sudo docker ps --filter "name=byparr" --format "{{.Names}}" | wc -l)
    if [ "$BYPARR_COUNT" -gt 0 ]; then
        echo "     ✅ byparr: $BYPARR_COUNT instance(s) running"
    else
        echo "     ❌ byparr: not running"
        ((ISSUES++))
    fi
    
    LB_STATUS=$(sudo docker inspect byparr-lb --format='{{.State.Status}}' 2>/dev/null || echo "not found")
    if [ "$LB_STATUS" = "running" ]; then
        echo "     ✅ byparr-lb: running"
    else
        echo "     ❌ byparr-lb: $LB_STATUS"
        ((ISSUES++))
    fi
else
    echo "  ❌ No containers found"
    ((ISSUES++))
fi

# Check 4: Byparr Health
echo "[4/10] Checking Byparr health..."
BYPARR_RESPONSE=$(curl -s -m 5 http://localhost:8191/v1 2>/dev/null)
if echo "$BYPARR_RESPONSE" | grep -q "FlareSolverr"; then
    echo "  ✅ Byparr is responding"
else
    echo "  ❌ Byparr not responding at http://localhost:8191"
    ((ISSUES++))
fi

# Check 5: Settings File
echo "[5/10] Checking settings.json..."
if [ -f "conf/settings.json" ]; then
    echo "  ✅ Settings file exists"
    
    # Check cookies
    if grep -q '"cookies"' conf/settings.json; then
        COOKIE_LENGTH=$(grep -o '"cookies":"[^"]*"' conf/settings.json | wc -c)
        echo "     📊 Cookie length: $COOKIE_LENGTH characters"
        
        if grep -q '"cookies":""' conf/settings.json || grep -q '"cookies": ""' conf/settings.json; then
            echo "     ❌ Cookies are empty"
            ((ISSUES++))
        elif [ "$COOKIE_LENGTH" -lt 100 ]; then
            echo "     ⚠️  Cookies seem too short"
            ((WARNINGS++))
        else
            if grep -q "cf_clearance=" conf/settings.json; then
                echo "     ✅ cf_clearance cookie found"
            else
                echo "     ❌ cf_clearance cookie missing"
                ((ISSUES++))
            fi
            
            if grep -q "csrftoken=" conf/settings.json; then
                echo "     ✅ csrftoken cookie found"
            else
                echo "     ⚠️  csrftoken cookie missing"
                ((WARNINGS++))
            fi
        fi
    else
        echo "     ❌ No cookies field found"
        ((ISSUES++))
    fi
    
    # Check user agent
    if grep -q '"user_agent"' conf/settings.json && ! grep -q '"user_agent":""' conf/settings.json; then
        echo "     ✅ User-Agent configured"
    else
        echo "     ⚠️  User-Agent not configured"
        ((WARNINGS++))
    fi
else
    echo "  ❌ Settings file not found"
    ((ISSUES++))
fi

# Check 6: Channels Configuration
echo "[6/10] Checking channels.json..."
if [ -f "conf/channels.json" ]; then
    TOTAL_CHANNELS=$(jq '. | length' conf/channels.json 2>/dev/null || echo "0")
    ACTIVE_CHANNELS=$(jq '[.[] | select(.is_paused == false)] | length' conf/channels.json 2>/dev/null || echo "0")
    
    echo "  ✅ Channels file exists"
    echo "     📊 Total channels: $TOTAL_CHANNELS"
    echo "     📊 Active channels: $ACTIVE_CHANNELS"
    echo "     📊 Paused channels: $((TOTAL_CHANNELS - ACTIVE_CHANNELS))"
    
    if [ "$ACTIVE_CHANNELS" -eq 0 ]; then
        echo "     ⚠️  No active channels"
        ((WARNINGS++))
    fi
else
    echo "  ⚠️  Channels file not found"
    ((WARNINGS++))
fi

# Check 7: Recent Logs
echo "[7/10] Checking recent logs..."
RECENT_ERRORS=$(sudo docker logs goondvr --tail 100 2>/dev/null | grep -c "ERROR" || echo "0")
CF_BLOCKS=$(sudo docker logs goondvr --tail 100 2>/dev/null | grep -c "Cloudflare" || echo "0")
OFFLINE_COUNT=$(sudo docker logs goondvr --tail 100 2>/dev/null | grep -c "channel is offline" || echo "0")

echo "  📊 Recent errors: $RECENT_ERRORS"
echo "  📊 Cloudflare blocks: $CF_BLOCKS"
echo "  📊 Offline checks: $OFFLINE_COUNT"

if [ "$CF_BLOCKS" -gt 5 ]; then
    echo "     ❌ High Cloudflare block rate - need fresh cookies"
    ((ISSUES++))
elif [ "$CF_BLOCKS" -gt 0 ]; then
    echo "     ⚠️  Some Cloudflare blocks detected"
    ((WARNINGS++))
fi

# Check 8: Disk Space
echo "[8/10] Checking disk space..."
DISK_USAGE=$(df -h / | tail -1 | awk '{print $5}' | sed 's/%//')
DISK_AVAIL=$(df -h / | tail -1 | awk '{print $4}')

echo "  📊 Disk usage: ${DISK_USAGE}%"
echo "  📊 Available: $DISK_AVAIL"

if [ "$DISK_USAGE" -gt 90 ]; then
    echo "     ❌ Disk space critical"
    ((ISSUES++))
elif [ "$DISK_USAGE" -gt 80 ]; then
    echo "     ⚠️  Disk space warning"
    ((WARNINGS++))
fi

# Check 9: Memory Usage
echo "[9/10] Checking memory..."
MEM_USAGE=$(free | grep Mem | awk '{printf "%.0f", $3/$2 * 100}')
MEM_AVAIL=$(free -h | grep Mem | awk '{print $7}')

echo "  📊 Memory usage: ${MEM_USAGE}%"
echo "  📊 Available: $MEM_AVAIL"

if [ "$MEM_USAGE" -gt 90 ]; then
    echo "     ❌ Memory critical"
    ((ISSUES++))
elif [ "$MEM_USAGE" -gt 80 ]; then
    echo "     ⚠️  Memory warning"
    ((WARNINGS++))
fi

# Check 10: Active Recordings
echo "[10/10] Checking active recordings..."
ACTIVE_RECORDINGS=$(sudo docker exec goondvr sh -c 'ls -1 /usr/src/app/videos/*.ts /usr/src/app/videos/*.mp4 2>/dev/null | wc -l' 2>/dev/null || echo "0")

echo "  📊 Active recordings: $ACTIVE_RECORDINGS"

if [ "$ACTIVE_RECORDINGS" -gt 0 ]; then
    echo "     ✅ Channels are recording!"
    sudo docker exec goondvr sh -c 'ls -lh /usr/src/app/videos/*.ts /usr/src/app/videos/*.mp4 2>/dev/null | tail -5'
else
    echo "     ⚠️  No active recordings"
    ((WARNINGS++))
fi

# Summary
echo ""
echo "====================================="
echo "📊 DIAGNOSTIC SUMMARY"
echo "====================================="
echo ""

if [ "$ISSUES" -eq 0 ] && [ "$WARNINGS" -eq 0 ]; then
    echo "✅ All checks passed!"
    echo ""
    echo "If channels still aren't recording:"
    echo "  1. Check if channels are actually online at chaturbate.com"
    echo "  2. View detailed logs: sudo docker logs -f goondvr"
    echo "  3. Check web UI: http://$(curl -s http://169.254.169.254/latest/meta-data/public-ipv4):8080"
else
    if [ "$ISSUES" -gt 0 ]; then
        echo "❌ CRITICAL ISSUES: $ISSUES"
    fi
    if [ "$WARNINGS" -gt 0 ]; then
        echo "⚠️  WARNINGS: $WARNINGS"
    fi
    echo ""
    echo "🔧 RECOMMENDED ACTIONS:"
    echo ""
    
    if [ "$CF_BLOCKS" -gt 0 ] || grep -q '"cookies":""' conf/settings.json 2>/dev/null; then
        echo "1. UPDATE COOKIES (MOST IMPORTANT):"
        echo "   • Visit https://chaturbate.com in browser"
        echo "   • Press F12 → Network tab → Refresh"
        echo "   • Copy Cookie header from any request"
        echo "   • Edit: nano conf/settings.json"
        echo "   • Restart: sudo docker restart goondvr"
        echo ""
    fi
    
    if [ "$RECORDER_STATUS" != "running" ] || [ "$BYPARR_COUNT" -eq 0 ]; then
        echo "2. RESTART CONTAINERS:"
        echo "   sudo docker compose restart"
        echo ""
    fi
    
    if [ "$DISK_USAGE" -gt 80 ]; then
        echo "3. CLEAN UP DISK:"
        echo "   ./scripts/cleanup-disk.sh"
        echo "   sudo docker system prune -af"
        echo ""
    fi
fi

echo "====================================="
echo ""
echo "📚 For detailed solutions:"
echo "   cat AWS_RECORDING_FIX.md"
echo ""
echo "📊 View logs:"
echo "   sudo docker logs -f goondvr"
echo ""
echo "🌐 Web UI:"
PUBLIC_IP=$(curl -s http://169.254.169.254/latest/meta-data/public-ipv4 2>/dev/null || echo "UNKNOWN")
echo "   http://${PUBLIC_IP}:8080"
echo ""
