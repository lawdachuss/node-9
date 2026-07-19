package server

import (
	"log"
	"math/rand"
	"strings"
	"time"

	"github.com/teacat/chaturbate-dvr/database"
	"github.com/teacat/chaturbate-dvr/entity"
	"github.com/teacat/chaturbate-dvr/uploader"
)

const (
	// mediaWatcherInterval is how long the watcher sleeps between scan cycles.
	// A short interval keeps media back-fill near-real-time while the per-host
	// adaptive rate limiter (see uploader/ratelimit.go) prevents host API storms
	// even with parallelism + the watcher running on every node.
	mediaWatcherInterval = 60 * time.Second

	// mediaWatcherInitialDelay lets process startup finish before the first scan.
	mediaWatcherInitialDelay = 30 * time.Second

	// mediaWatcherScanLimit caps how many of the newest recordings we consider
	// per cycle. Hosts generate poster/preview within minutes of upload (and
	// never for very short clips), so only recent recordings are worth checking;
	// older ones that already have media are skipped, and ones that never got
	// media stay near the tail of the window until rotated out.
	mediaWatcherScanLimit = 2000
)

// StartMediaWatcher launches a background goroutine that continuously checks
// SeekStreaming and UPnShare for poster/preview URLs of our recordings and
// patches the Supabase recordings row once they become available.
//
// Why a central watcher instead of per-upload polling: the hosts generate the
// poster (.png) and preview (.webp) well after upload — the preview can arrive
// minutes (or much longer) later, and for very short clips it may never appear.
// A central, rotating scan keeps uploads fast (no blocking, no goroutine-per-
// recording) while still back-filling media for every recording we have.
//
// Call once during startup. It returns immediately; the work happens in the
// goroutine until `stop` is closed.
func StartMediaWatcher(stop <-chan struct{}) {
	go func() {
		defer func() {
			if r := recover(); r != nil {
				log.Printf("[MEDIA] PANIC in media watcher: %v", r)
			}
		}()

		ticker := time.NewTicker(mediaWatcherInterval)
		defer ticker.Stop()

		// Small jitter so multiple nodes don't all begin scanning in lockstep.
		select {
		case <-stop:
			return
		case <-time.After(mediaWatcherInitialDelay + time.Duration(randIntn(15))*time.Second):
		}

		for {
			select {
			case <-stop:
				return
			default:
				runMediaWatchCycle()
			}
			select {
			case <-stop:
				return
			case <-ticker.C:
			}
		}
	}()
}

// mediaWatcherCursor rotates through the scan window across cycles so a large
// backlog is covered over several passes rather than all at once.
var mediaWatcherCursor int

// runMediaWatchCycle performs one pass over the newest recordings, back-filling
// any poster/preview URLs the hosts have generated since the last pass.
func runMediaWatchCycle() {
	cfg := Config
	if cfg == nil || cfg.SupabaseURL == "" || cfg.SupabaseAPIKey == "" {
		return
	}
	if cfg.SeekStreamingKey == "" && len(cfg.UpnshareKeys) == 0 {
		return
	}

	client := GetDBClient()
	if client == nil {
		return
	}

	recs, err := client.GetNewestRecordings(mediaWatcherScanLimit)
	if err != nil {
		log.Printf("[MEDIA] failed to load recordings: %v", err)
		return
	}

	// Build the work list: recordings still missing poster or preview, with the
	// correct host resolved from the stored embed URL.
	type job struct {
		rec     *database.Recording
		host    string // "seekstreaming" | "upnshare"
		videoID string
	}
	var jobs []job
	for i := range recs {
		r := &recs[i]
		if r.ThumbnailURL != "" && r.PreviewURL != "" {
			continue
		}
		host, id := mediaHostOf(r.EmbedURL)
		if host == "" || id == "" {
			continue
		}
		jobs = append(jobs, job{rec: r, host: host, videoID: id})
	}

	if len(jobs) == 0 {
		return
	}

	// Rotate the starting offset so a large backlog is covered over time.
	start := 0
	if len(jobs) > 1 {
		start = mediaWatcherCursor % len(jobs)
		mediaWatcherCursor = (start + mediaWatcherScanLimit) % len(jobs)
	}

	// Single worker. The hosts we query (SeekStreaming/UPnShare) are shared and
	// rate-limited per host by uploader/ratelimit.go, so fanning out multiple
	// goroutines against one host would just serialize behind that limiter and
	// add confusion. A single sequential pass paces naturally; "speed" comes
	// from the tight per-host spacing + host discrimination (we only query the
	// one backend each recording actually lives on) + the short cycle interval.
	patched, checked, scanErrors := 0, 0, 0
	n := len(jobs)
	for k := 0; k < n; k++ {
		j := jobs[(start+k)%n]

		thumb, prev, err := fetchHostMedia(cfg, j.host, j.videoID, j.rec.Filename)
		if err != nil {
			// A host fetch error here almost always means "media not generated
			// yet" (404/400 from the manage API) rather than a real failure.
			// Treat it as "not ready", not a hard error, and move on. Genuine
			// transport failures are rare and self-heal on the next cycle.
			scanErrors++
			continue
		}

		newThumb := thumb != "" && j.rec.ThumbnailURL == ""
		newPrev := prev != "" && j.rec.PreviewURL == ""
		checked++

		if !newThumb && !newPrev {
			continue
		}

		if j.rec.ThumbnailURL == "" {
			j.rec.ThumbnailURL = thumb
		}
		if j.rec.PreviewURL == "" {
			j.rec.PreviewURL = prev
		}

		if err := UpdateRecordingMediaURLs(j.rec.Filename, j.rec.ThumbnailURL, j.rec.PreviewURL); err != nil {
			log.Printf("[MEDIA] failed to update %s: %v", j.rec.Filename, err)
			continue
		}
		patched++
		log.Printf("[MEDIA] updated %s (thumb=%v preview=%v)", j.rec.Filename, newThumb, newPrev)
	}

	if patched > 0 || scanErrors > 0 {
		log.Printf("[MEDIA] cycle done: checked=%d patched=%d not-ready=%d", checked, patched, scanErrors)
	}
}

// fetchHostMedia fetches poster/preview for a single video from the correct
// host backend. Only the host implied by the recording's embed URL is queried,
// which avoids wasteful (and potentially wrong) cross-backend lookups.
func fetchHostMedia(cfg *entity.Config, host, videoID, filename string) (thumb, prev string, err error) {
	switch host {
	case "seekstreaming":
		if cfg.SeekStreamingKey == "" {
			return "", "", nil
		}
		return uploader.GetSeekStreamingMediaURLs(cfg.SeekStreamingKey, videoID)
	case "upnshare":
		if len(cfg.UpnshareKeys) == 0 {
			return "", "", nil
		}
		// UPnShare is the same backend advertised under several player domains;
		// the first configured key is sufficient for the manage API.
		return uploader.GetUPnShareMediaURLs(cfg.UpnshareKeys[0], videoID, filename)
	default:
		return "", "", nil
	}
}

// mediaHostOf resolves which video host a recording's embed URL points at and
// returns the host tag plus the video ID (the fragment after '#').
//
//	SeekStreaming -> https://chuglii.seeks.cloud/#<id>  (or *.seekstreaming.info)
//	UPnShare      -> https://<prefix>.upns.online/#<id>
// randIntn is a small helper for jitter (math/rand is fine here; we don't need
// cryptographic randomness for timing jitter).
func randIntn(n int) int {
	if n <= 0 {
		return 0
	}
	return rand.Intn(n)
}

func mediaHostOf(embedURL string) (host, videoID string) {
	if idx := strings.LastIndex(embedURL, "#"); idx >= 0 {
		videoID = embedURL[idx+1:]
	}
	// Normalize the host portion (everything before the fragment).
	hostPart := embedURL
	if idx := strings.LastIndex(embedURL, "#"); idx >= 0 {
		hostPart = embedURL[:idx]
	}
	hostPart = strings.ToLower(hostPart)

	switch {
	case strings.Contains(hostPart, "seekstreaming") || strings.Contains(hostPart, "seeks.cloud"):
		return "seekstreaming", videoID
	case strings.Contains(hostPart, "upns") || strings.Contains(hostPart, "upnshare"):
		// Covers *.upns.online and upnshare.com player domains.
		return "upnshare", videoID
	}
	return "", videoID
}
