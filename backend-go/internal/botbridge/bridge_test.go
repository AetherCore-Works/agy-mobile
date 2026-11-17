package botbridge

import (
	"testing"

	"github.com/Phoenix7290/agy-mobile-gateway/backend-go/internal/config"
)

func TestVerifyBearer(t *testing.T) {
	if VerifyBearer("Bearer secret123", "") {
		t.Fatal("empty expected secret must fail")
	}
	if !VerifyBearer("Bearer secret123", "secret123") {
		t.Fatal("matching bearer token must pass")
	}
	if VerifyBearer("Bearer wrong", "secret123") {
		t.Fatal("mismatched bearer token must fail")
	}
	if VerifyBearer("Basic secret123", "secret123") {
		t.Fatal("non-bearer scheme must fail")
	}
}

func TestAllowlists(t *testing.T) {
	b := New(config.GatewaySettings{
		WhatsAppAllowedNumbers: []string{"558398677675"},
		DiscordAllowedUserIDs:  []string{"605488871773437993"},
	}, nil, nil)

	if _, ok := b.IsAllowedWhatsApp("558398677675@s.whatsapp.net", true); ok {
		t.Fatal("must ignore fromMe messages")
	}
	if _, ok := b.IsAllowedWhatsApp("120363000000@g.us", false); ok {
		t.Fatal("must ignore group messages")
	}
	if _, ok := b.IsAllowedWhatsApp("5511999999999@s.whatsapp.net", false); ok {
		t.Fatal("must reject unlisted phone number")
	}
	if num, ok := b.IsAllowedWhatsApp("558398677675@s.whatsapp.net", false); !ok || num != "558398677675" {
		t.Fatalf("expected allowed whatsapp number, got num=%q ok=%v", num, ok)
	}

	if b.IsAllowedDiscordUser("605488871773437993", false) {
		t.Fatal("must reject non-DM discord message")
	}
	if b.IsAllowedDiscordUser("999999999999999999", true) {
		t.Fatal("must reject unlisted discord user")
	}
	if !b.IsAllowedDiscordUser("605488871773437993", true) {
		t.Fatal("must allow configured discord user in DM")
	}
}
