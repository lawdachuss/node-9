package internal

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// FormatDuration converts a float64 duration (in seconds) to h:m:s format.
func FormatDuration(duration float64) string {
	if duration == 0 {
		return ""
	}
	var (
		hours   = int(duration) / 3600
		minutes = (int(duration) % 3600) / 60
		seconds = int(duration) % 60
	)
	return fmt.Sprintf("%d:%02d:%02d", hours, minutes, seconds)
}

// FormatFilesize converts an int64 filesize in bytes to a human-readable string (KB, MB, GB).
func FormatFilesize(filesize int64) string {
	if filesize == 0 {
		return ""
	}
	const (
		KB int64 = 1024
		MB       = KB * 1024
		GB       = MB * 1024
	)
	switch {
	case filesize >= GB:
		return fmt.Sprintf("%.2f GB", float64(filesize)/float64(GB))
	case filesize >= MB:
		return fmt.Sprintf("%.2f MB", float64(filesize)/float64(MB))
	case filesize >= KB:
		return fmt.Sprintf("%.2f KB", float64(filesize)/float64(KB))
	default:
		return fmt.Sprintf("%d bytes", filesize)
	}
}

// SegmentSeq extracts the segment sequence number from a URI.
// Handles both legacy .ts segments (e.g. chunk_12345.ts) and
// LL-HLS .m4s segments (e.g. seg_4_10879_video_..._llhls.m4s).
func SegmentSeq(filename string) int {
	// Strip query string before matching
	clean := filename
	if idx := strings.Index(clean, "?"); idx >= 0 {
		clean = clean[:idx]
	}
	// LL-HLS fMP4: seg_<id>_<seq>_<type>_..._llhls.m4s
	if strings.HasSuffix(clean, ".m4s") {
		re := regexp.MustCompile(`seg_\d+_(\d+)_`)
		if match := re.FindStringSubmatch(clean); len(match) > 1 {
			if n, err := strconv.Atoi(match[1]); err == nil {
				return n
			}
		}
	}
	// Legacy HLS: _<seq>.ts
	re := regexp.MustCompile(`_(\d+)\.ts$`)
	if match := re.FindStringSubmatch(clean); len(match) > 1 {
		if n, err := strconv.Atoi(match[1]); err == nil {
			return n
		}
	}
	return -1
}
