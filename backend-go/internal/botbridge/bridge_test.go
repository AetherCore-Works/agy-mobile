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
		WhatsAppAllowedNumbers: []string{"5500999990000"},
		DiscordAllowedUserIDs:  []string{"111111111111111111"},
	}, nil, nil)

	if _, ok := b.IsAllowedWhatsApp("5500999990000@s.whatsapp.net", true); ok {
		t.Fatal("must ignore fromMe messages")
	}
	if _, ok := b.IsAllowedWhatsApp("120363000000@g.us", false); ok {
		t.Fatal("must ignore group messages")
	}
	if _, ok := b.IsAllowedWhatsApp("5511999999999@s.whatsapp.net", false); ok {
		t.Fatal("must reject unlisted phone number")
	}
	if num, ok := b.IsAllowedWhatsApp("5500999990000@s.whatsapp.net", false); !ok || num != "5500999990000" {
		t.Fatalf("expected allowed whatsapp number, got num=%q ok=%v", num, ok)
	}

	if b.IsAllowedDiscordUser("111111111111111111", false) {
		t.Fatal("must reject non-DM discord message")
	}
	if b.IsAllowedDiscordUser("999999999999999999", true) {
		t.Fatal("must reject unlisted discord user")
	}
	if !b.IsAllowedDiscordUser("111111111111111111", true) {
		t.Fatal("must allow configured discord user in DM")
	}
}

func TestFormattingAndChunking(t *testing.T) {
	wa := FormatForWhatsApp("## Status\nTudo **online** no servidor.")
	if wa != "*Status*\nTudo *online* no servidor." {
		t.Fatalf("unexpected WhatsApp formatting: %q", wa)
	}
	chunks := SplitChunks("linha1\nlinha2\nlinha3", 12)
	if len(chunks) < 2 {
		t.Fatalf("expected multi-chunk split, got %v", chunks)
	}
}
