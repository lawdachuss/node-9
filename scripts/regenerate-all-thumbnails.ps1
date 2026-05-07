# Regenerate thumbnails for all videos in Supabase
# Usage: .\scripts\regenerate-all-thumbnails.ps1

$ErrorActionPreference = "Stop"

Write-Host "╔════════════════════════════════════════════════════════════╗" -ForegroundColor Cyan
Write-Host "║        📸 REGENERATE ALL THUMBNAILS                       ║" -ForegroundColor Cyan
Write-Host "╚════════════════════════════════════════════════════════════╝" -ForegroundColor Cyan
Write-Host ""

# Check if ffmpeg is installed
try {
    $null = ffmpeg -version 2>&1
} catch {
    Write-Host "❌ ffmpeg is not installed" -ForegroundColor Red
    Write-Host "Download from: https://ffmpeg.org/download.html" -ForegroundColor Yellow
    exit 1
}

# Load environment variables
if (Test-Path ".env") {
    Get-Content ".env" | ForEach-Object {
        if ($_ -match '^\s*([^#][^=]+)=(.*)$') {
            $name = $matches[1].Trim()
            $value = $matches[2].Trim()
            [Environment]::SetEnvironmentVariable($name, $value, "Process")
        }
    }
}

$SUPABASE_URL = $env:SUPABASE_URL
$SUPABASE_API_KEY = $env:SUPABASE_API_KEY

if (-not $SUPABASE_URL -or -not $SUPABASE_API_KEY) {
    Write-Host "❌ SUPABASE_URL and SUPABASE_API_KEY must be set" -ForegroundColor Red
    exit 1
}

Write-Host "📡 Fetching videos from Supabase..." -ForegroundColor Yellow

try {
    $videos = Invoke-RestMethod -Uri "$SUPABASE_URL/rest/v1/video_uploads?select=*" `
        -Method Get `
        -Headers @{
            "apikey" = $SUPABASE_API_KEY
            "Content-Type" = "application/json"
        }
    
    Write-Host "✅ Found $($videos.Count) videos" -ForegroundColor Green
    Write-Host ""
    
    $noThumbnail = $videos | Where-Object { -not $_.thumbnail_link -or $_.thumbnail_link -eq "" }
    
    Write-Host "⚠️  $($noThumbnail.Count) videos without thumbnails" -ForegroundColor Yellow
    Write-Host ""
    
    if ($noThumbnail.Count -eq 0) {
        Write-Host "✅ All videos already have thumbnails!" -ForegroundColor Green
        exit 0
    }
    
    Write-Host "This script needs access to the original video files to generate thumbnails." -ForegroundColor White
    Write-Host "Video files should be in the 'videos/completed' directory." -ForegroundColor White
    Write-Host ""
    
    $continue = Read-Host "Continue? (y/n)"
    if ($continue -ne "y") {
        Write-Host "Cancelled" -ForegroundColor Yellow
        exit 0
    }
    
    Write-Host ""
    Write-Host "Starting thumbnail generation..." -ForegroundColor Cyan
    Write-Host ""
    
    $generated = 0
    $failed = 0
    $total = $noThumbnail.Count
    
    foreach ($video in $noThumbnail) {
        $current = $generated + $failed + 1
        Write-Host "[$current/$total] Processing: $($video.filename)" -ForegroundColor White
        
        # Try to find the video file
        $videoPath = $null
        if (Test-Path "videos/completed/$($video.filename)") {
            $videoPath = "videos/completed/$($video.filename)"
        } elseif (Test-Path "videos/$($video.filename)") {
            $videoPath = "videos/$($video.filename)"
        }
        
        if (-not $videoPath) {
            Write-Host "  ❌ Video file not found, skipping" -ForegroundColor Red
            $failed++
            continue
        }
        
        # Generate thumbnail
        $thumbPath = [System.IO.Path]::Combine($env:TEMP, "$([System.IO.Path]::GetFileNameWithoutExtension($video.filename))_thumb.jpg")
        if (Test-Path $thumbPath) {
            Remove-Item $thumbPath -Force
        }
        
        Write-Host "  📸 Generating thumbnail..." -ForegroundColor Yellow
        
        # Try with seeking first
        $ffmpegArgs = @("-y", "-ss", "2", "-i", $videoPath, "-vframes", "1", "-vf", "scale=640:-2", "-q:v", "2", $thumbPath)
        $process = Start-Process -FilePath "ffmpeg" -ArgumentList $ffmpegArgs -NoNewWindow -Wait -PassThru -RedirectStandardError "$env:TEMP\ffmpeg_error.txt"
        
        if ($process.ExitCode -ne 0) {
            # Try without seeking
            $ffmpegArgs = @("-y", "-i", $videoPath, "-vframes", "1", "-vf", "scale=640:-2", "-q:v", "2", $thumbPath)
            $process = Start-Process -FilePath "ffmpeg" -ArgumentList $ffmpegArgs -NoNewWindow -Wait -PassThru -RedirectStandardError "$env:TEMP\ffmpeg_error.txt"
        }
        
        if ($process.ExitCode -ne 0 -or -not (Test-Path $thumbPath)) {
            Write-Host "  ❌ Failed to generate thumbnail" -ForegroundColor Red
            $failed++
            continue
        }
        
        Write-Host "  ✅ Thumbnail generated" -ForegroundColor Green
        
        # Upload to Pixhost.to
        Write-Host "  ☁️  Uploading to Pixhost.to..." -ForegroundColor Yellow
        
        try {
            $boundary = [System.Guid]::NewGuid().ToString()
            $fileBytes = [System.IO.File]::ReadAllBytes($thumbPath)
            $fileName = [System.IO.Path]::GetFileName($thumbPath)
            
            $bodyLines = @(
                "--$boundary",
                "Content-Disposition: form-data; name=`"img`"; filename=`"$fileName`"",
                "Content-Type: image/jpeg",
                "",
                [System.Text.Encoding]::GetEncoding("iso-8859-1").GetString($fileBytes),
                "--$boundary",
                "Content-Disposition: form-data; name=`"content_type`"",
                "",
                "1",
                "--$boundary",
                "Content-Disposition: form-data; name=`"max_th_size`"",
                "",
                "420",
                "--$boundary--"
            )
            
            $body = $bodyLines -join "`r`n"
            
            $response = Invoke-RestMethod -Uri "https://api.pixhost.to/images" `
                -Method Post `
                -ContentType "multipart/form-data; boundary=$boundary" `
                -Body ([System.Text.Encoding]::GetEncoding("iso-8859-1").GetBytes($body)) `
                -Headers @{ "Accept" = "application/json" }
            
            $thumbUrl = $response.show_url
            
            if (-not $thumbUrl) {
                Write-Host "  ❌ Failed to upload thumbnail" -ForegroundColor Red
                Write-Host "  Response: $($response | ConvertTo-Json)" -ForegroundColor Gray
                $failed++
                Remove-Item $thumbPath -Force
                continue
            }
            
            Write-Host "  ✅ Uploaded: $thumbUrl" -ForegroundColor Green
            
            # Update Supabase
            Write-Host "  💾 Updating Supabase..." -ForegroundColor Yellow
            
            $updateResponse = Invoke-RestMethod -Uri "$SUPABASE_URL/rest/v1/video_uploads?id=eq.$($video.id)" `
                -Method Patch `
                -Headers @{
                    "apikey" = $SUPABASE_API_KEY
                    "Content-Type" = "application/json"
                    "Prefer" = "return=minimal"
                } `
                -Body (@{ thumbnail_link = $thumbUrl } | ConvertTo-Json)
            
            Write-Host "  ✅ Database updated" -ForegroundColor Green
            $generated++
            
        } catch {
            Write-Host "  ❌ Upload failed: $_" -ForegroundColor Red
            $failed++
        } finally {
            # Clean up
            if (Test-Path $thumbPath) {
                Remove-Item $thumbPath -Force
            }
        }
        
        Write-Host ""
    }
    
    Write-Host "════════════════════════════════════════════════════════════" -ForegroundColor Cyan
    Write-Host "COMPLETE" -ForegroundColor Cyan
    Write-Host "════════════════════════════════════════════════════════════" -ForegroundColor Cyan
    Write-Host ""
    Write-Host "✅ Generated: $generated" -ForegroundColor Green
    Write-Host "❌ Failed: $failed" -ForegroundColor Red
    Write-Host ""
    
} catch {
    Write-Host "❌ Error: $_" -ForegroundColor Red
    exit 1
}
