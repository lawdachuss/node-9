package channel

import (
	"testing"

	"github.com/teacat/chaturbate-dvr/entity"
	"github.com/teacat/chaturbate-dvr/server"
)

// TestConfiguredUploadHostsIncludesSeekStreaming is a regression test for a
// data-loss bug: configuredUploadHosts() previously maintained a hand-written
// list that omitted SeekStreaming.  IsAlreadyFullyUploaded() uses this list to
// decide whether the watcher may delete the local file, so when the other hosts
// succeeded but SeekStreaming had not, the watcher deleted the file and
// SeekStreaming never received it.
//
// The list must now exactly match uploader.NewMultiHostUploader(...).AvailableHosts().
func TestConfiguredUploadHostsIncludesSeekStreaming(t *testing.T) {
	oldConfig := server.Config
	defer func() { server.Config = oldConfig }()
	server.Config = &entity.Config{
		VoeSXAPIKey:        "key",
		StreamtapeLogin:    "user",
		StreamtapeKey:      "pass",
		MixdropEmail:       "a@b.c",
		MixdropToken:       "tok",
		SeekStreamingKey:   "ss-key",
		VidHideAPIKeys:     []string{"vh-key"},
		StreamWishAPIKeys:  []string{"sw-key"},
		UpnshareKeys:       []string{"us-key"},
	}

	hosts := configuredUploadHosts()
	has := func(name string) bool {
		for _, h := range hosts {
			if h == name {
				return true
			}
		}
		return false
	}
	for _, want := range []string{"GoFile", "VOE.sx", "Streamtape", "Mixdrop", "SeekStreaming", "VidHide", "StreamWish", "UPnShare"} {
		if !has(want) {
			t.Errorf("configuredUploadHosts() missing %q; got %v", want, hosts)
		}
	}
}

// TestConfiguredUploadHostsMinimal confirms the minimal case still works and
// that IsAlreadyFullyUploaded's "len(hosts) == 0 -> false" guard is never hit
// when only the always-available hosts (GoFile + PixelDrain) are present.
func TestConfiguredUploadHostsMinimal(t *testing.T) {
	oldConfig := server.Config
	defer func() { server.Config = oldConfig }()
	server.Config = &entity.Config{} // no API keys -> GoFile + PixelDrain

	hosts := configuredUploadHosts()
	has := func(name string) bool {
		for _, h := range hosts {
			if h == name {
				return true
			}
		}
		return false
	}
	for _, want := range []string{"GoFile", "PixelDrain"} {
		if !has(want) {
			t.Fatalf("expected %q in hosts, got %v", want, hosts)
		}
	}
	if len(hosts) != 2 {
		t.Fatalf("expected exactly [GoFile PixelDrain], got %v", hosts)
	}
}
