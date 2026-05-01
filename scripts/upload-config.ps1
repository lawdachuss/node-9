# Upload Configuration to EC2
# This script uploads your local configuration files to the EC2 instance

param(
    [string]$EC2Host = "32.193.245.111",
    [string]$KeyPath = "aws-secrets/aws-key.pem",
    [string]$AppDir = "/home/ubuntu/goondvr"
)

$ErrorActionPreference = "Stop"

Write-Host "Uploading Configuration to EC2" -ForegroundColor Cyan
Write-Host "===================================" -ForegroundColor Cyan
Write-Host ""

# Check if key file exists
if (-not (Test-Path $KeyPath)) {
    Write-Host "ERROR: SSH key not found at: $KeyPath" -ForegroundColor Red
    exit 1
}

# Check if conf directory exists
if (-not (Test-Path "conf")) {
    Write-Host "ERROR: conf/ directory not found" -ForegroundColor Red
    exit 1
}

Write-Host "Found configuration files:" -ForegroundColor Green
Get-ChildItem conf/*.json | ForEach-Object {
    Write-Host "  - $($_.Name)" -ForegroundColor Gray
}
Write-Host ""

# Upload settings.json
if (Test-Path "conf/settings.json") {
    Write-Host "Uploading settings.json..." -ForegroundColor Yellow
    scp -i $KeyPath conf/settings.json ubuntu@${EC2Host}:/tmp/settings.json
    ssh -i $KeyPath ubuntu@$EC2Host "sudo mv /tmp/settings.json $AppDir/conf/settings.json && sudo chown ubuntu:ubuntu $AppDir/conf/settings.json"
    Write-Host "SUCCESS: settings.json uploaded" -ForegroundColor Green
} else {
    Write-Host "WARNING: settings.json not found, skipping" -ForegroundColor Yellow
}

# Upload channels.json
if (Test-Path "conf/channels.json") {
    Write-Host "Uploading channels.json..." -ForegroundColor Yellow
    scp -i $KeyPath conf/channels.json ubuntu@${EC2Host}:/tmp/channels.json
    ssh -i $KeyPath ubuntu@$EC2Host "sudo mv /tmp/channels.json $AppDir/conf/channels.json && sudo chown ubuntu:ubuntu $AppDir/conf/channels.json"
    Write-Host "SUCCESS: channels.json uploaded" -ForegroundColor Green
} else {
    Write-Host "WARNING: channels.json not found, skipping" -ForegroundColor Yellow
}

Write-Host ""
Write-Host "Restarting application..." -ForegroundColor Yellow
ssh -i $KeyPath ubuntu@$EC2Host "cd $AppDir && sudo docker compose restart recorder"

Write-Host ""
Write-Host "SUCCESS: Configuration uploaded and application restarted!" -ForegroundColor Green
Write-Host ""
Write-Host "Check status:" -ForegroundColor Cyan
Write-Host "  ssh -i $KeyPath ubuntu@$EC2Host 'cd $AppDir && sudo docker compose logs recorder --tail=20'" -ForegroundColor Gray
Write-Host ""
Write-Host "Web UI: http://$EC2Host:8080" -ForegroundColor Cyan
