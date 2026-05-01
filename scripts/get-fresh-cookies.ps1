# Get Fresh Cookies from Browser for Chaturbate Recording
# This script helps you extract cookies from your browser

Write-Host "🍪 Chaturbate Cookie Extractor" -ForegroundColor Cyan
Write-Host "================================" -ForegroundColor Cyan
Write-Host ""

Write-Host "📋 INSTRUCTIONS:" -ForegroundColor Yellow
Write-Host "1. Open Chrome or Firefox" -ForegroundColor White
Write-Host "2. Go to https://chaturbate.com" -ForegroundColor White
Write-Host "3. Press F12 to open Developer Tools" -ForegroundColor White
Write-Host "4. Click the 'Network' tab" -ForegroundColor White
Write-Host "5. Refresh the page (F5)" -ForegroundColor White
Write-Host "6. Click on any request to 'chaturbate.com'" -ForegroundColor White
Write-Host "7. Scroll down to 'Request Headers'" -ForegroundColor White
Write-Host "8. Find the 'Cookie:' header" -ForegroundColor White
Write-Host "9. Right-click and 'Copy value'" -ForegroundColor White
Write-Host ""

Write-Host "Press Enter when you have copied the cookies..." -ForegroundColor Green
Read-Host

Write-Host ""
Write-Host "📝 Paste your cookies here (Ctrl+V then Enter):" -ForegroundColor Yellow
$cookies = Read-Host

if ([string]::IsNullOrWhiteSpace($cookies)) {
    Write-Host "❌ No cookies provided. Exiting." -ForegroundColor Red
    exit 1
}

# Clean the cookies (remove newlines, extra spaces)
$cookies = $cookies -replace "`r`n", "" -replace "`n", "" -replace "`r", ""
$cookies = $cookies.Trim()

Write-Host ""
Write-Host "🔍 Validating cookies..." -ForegroundColor Yellow

# Check for important cookies
$hasCfClearance = $cookies -match "cf_clearance="
$hasCsrfToken = $cookies -match "csrftoken="

if (-not $hasCfClearance) {
    Write-Host "⚠️  WARNING: No 'cf_clearance' cookie found!" -ForegroundColor Yellow
    Write-Host "   This cookie is required to bypass Cloudflare." -ForegroundColor Yellow
    Write-Host "   Make sure you copied the FULL cookie string." -ForegroundColor Yellow
}

if (-not $hasCsrfToken) {
    Write-Host "⚠️  WARNING: No 'csrftoken' cookie found!" -ForegroundColor Yellow
    Write-Host "   This cookie is required for API requests." -ForegroundColor Yellow
}

Write-Host ""
Write-Host "📝 Also paste your User-Agent here:" -ForegroundColor Yellow
Write-Host "   (Find it in the same Request Headers section)" -ForegroundColor Gray
$userAgent = Read-Host

if ([string]::IsNullOrWhiteSpace($userAgent)) {
    Write-Host "⚠️  No User-Agent provided. Using default." -ForegroundColor Yellow
    $userAgent = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36"
}

# Clean user agent
$userAgent = $userAgent -replace "`r`n", "" -replace "`n", "" -replace "`r", ""
$userAgent = $userAgent.Trim()

Write-Host ""
Write-Host "💾 Updating settings.json..." -ForegroundColor Yellow

# Read current settings
$settingsPath = "settings.json"
if (-not (Test-Path $settingsPath)) {
    $settingsPath = "conf/settings.json"
}

if (-not (Test-Path $settingsPath)) {
    Write-Host "❌ settings.json not found!" -ForegroundColor Red
    Write-Host "   Creating new settings file..." -ForegroundColor Yellow
    
    $settings = @{
        cookies = $cookies
        user_agent = $userAgent
        enable_gofile_upload = $true
        finalize_mode = "remux"
        ffmpeg_container = "mp4"
    }
} else {
    $settingsJson = Get-Content $settingsPath -Raw | ConvertFrom-Json
    $settings = @{}
    $settingsJson.PSObject.Properties | ForEach-Object {
        $settings[$_.Name] = $_.Value
    }
    
    # Update cookies and user agent
    $settings["cookies"] = $cookies
    $settings["user_agent"] = $userAgent
}

# Save settings
$settingsJson = $settings | ConvertTo-Json -Depth 10
$settingsJson | Set-Content $settingsPath -Encoding UTF8

Write-Host "✅ Settings updated successfully!" -ForegroundColor Green
Write-Host ""
Write-Host "📊 Cookie Summary:" -ForegroundColor Cyan
Write-Host "   Length: $($cookies.Length) characters" -ForegroundColor White
Write-Host "   cf_clearance: $(if ($hasCfClearance) { '✅ Found' } else { '❌ Missing' })" -ForegroundColor White
Write-Host "   csrftoken: $(if ($hasCsrfToken) { '✅ Found' } else { '❌ Missing' })" -ForegroundColor White
Write-Host ""

Write-Host "🚀 Next Steps:" -ForegroundColor Yellow
Write-Host "1. Run the recorder: .\goondvr.exe --debug" -ForegroundColor White
Write-Host "2. Check the web UI: http://localhost:8080" -ForegroundColor White
Write-Host "3. Monitor logs for 'channel is online' messages" -ForegroundColor White
Write-Host ""

Write-Host "⏰ Note: Cookies expire after 24-48 hours" -ForegroundColor Yellow
Write-Host "   You'll need to refresh them periodically" -ForegroundColor Yellow
Write-Host ""

Write-Host "Press Enter to exit..." -ForegroundColor Green
Read-Host
