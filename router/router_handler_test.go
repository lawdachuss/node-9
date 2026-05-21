package router

import "testing"

func TestBuildHostPlayersGeneratesEmbeds(t *testing.T) {
	players := buildHostPlayers(map[string]string{
		"Byse":   "https://byse.sx/d/2stlap4n6a0h",
		"GoFile": "https://gofile.io/d/example",
		"SendCM": "https://send.now/sendcode123",
		"VOE.sx": "https://voe.sx/abc123",
	}, "")

	byHost := map[string]hostPlayer{}
	for _, player := range players {
		byHost[player.Host] = player
	}

	if got := byHost["Byse"].EmbedURL; got != "https://api.byse.sx/e/2stlap4n6a0h" {
		t.Fatalf("Byse embed URL = %q", got)
	}
	if got := byHost["Byse"].VideoURL; got != "" {
		t.Fatalf("Byse video URL = %q", got)
	}
	if got := byHost["GoFile"].EmbedURL; got != "" {
		t.Fatalf("GoFile embed URL = %q", got)
	}
	if got := byHost["GoFile"].VideoURL; got != "" {
		t.Fatalf("GoFile video URL = %q", got)
	}
	if got := byHost["SendCM"].EmbedURL; got != "https://send.now/sendcode123" {
		t.Fatalf("SendCM embed URL = %q", got)
	}
	if got := byHost["VOE.sx"].EmbedURL; got != "https://voe.sx/e/abc123" {
		t.Fatalf("VOE embed URL = %q", got)
	}
}
