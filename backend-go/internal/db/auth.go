package db

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"database/sql"
	"encoding/hex"
	"fmt"
	"strings"
)

// WebAuthnCredential stores a registered ES256 (P-256) biometric/passkey public key.
type WebAuthnCredential struct {
	ID           string `json:"id"`
	CredentialID string `json:"credential_id"`
	PublicKeyX   string `json:"public_key_x"`
	PublicKeyY   string `json:"public_key_y"`
	SignCount    uint32 `json:"sign_count"`
	Label        string `json:"label"`
	CreatedAt    string `json:"created_at"`
}

func (g *GatewayDB) getInternalSetting(key string) (string, error) {
	g.mu.Lock()
	defer g.mu.Unlock()

	var raw string
	err := g.sql.QueryRow("SELECT value FROM settings WHERE key = ?", key).Scan(&raw)
	if err == sql.ErrNoRows {
		return "", nil
	}
	return raw, err
}

func (g *GatewayDB) setInternalSetting(key, val string) error {
	g.mu.Lock()
	defer g.mu.Unlock()

	if val == "" {
		_, err := g.sql.Exec("DELETE FROM settings WHERE key = ?", key)
		return err
	}
	_, err := g.sql.Exec(
		`INSERT INTO settings (key, value, updated_at)
		 VALUES (?, ?, ?)
		 ON CONFLICT(key) DO UPDATE SET value = excluded.value, updated_at = excluded.updated_at`,
		key, val, utcNow(),
	)
	return err
}

// GetOrCreateAuthSecret returns a persistent 32-byte signing key stored in SQLite for HttpOnly session cookies.
func (g *GatewayDB) GetOrCreateAuthSecret() ([]byte, error) {
	raw, err := g.getInternalSetting("_auth_cookie_secret")
	if err != nil {
		return nil, err
	}
	if raw != "" {
		if decoded, err := hex.DecodeString(strings.TrimSpace(raw)); err == nil && len(decoded) >= 32 {
			return decoded, nil
		}
	}
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return nil, err
	}
	if err := g.setInternalSetting("_auth_cookie_secret", hex.EncodeToString(b)); err != nil {
		return nil, err
	}
	return b, nil
}

// HasAuthPIN reports whether a custom UI PIN/password hash is stored in SQLite.
func (g *GatewayDB) HasAuthPIN() bool {
	raw, err := g.getInternalSetting("_auth_pin_hash")
	return err == nil && strings.TrimSpace(raw) != ""
}

func hashSaltedPIN(saltHex, pin string) string {
	sum := sha256.Sum256([]byte(saltHex + ":" + pin))
	return hex.EncodeToString(sum[:])
}

// SetAuthPIN stores a salted SHA-256 hash of pin, or clears it when pin is empty.
func (g *GatewayDB) SetAuthPIN(pin string) error {
	trimmed := strings.TrimSpace(pin)
	if trimmed == "" {
		return g.setInternalSetting("_auth_pin_hash", "")
	}
	if len(trimmed) < 4 {
		return fmt.Errorf("PIN/senha deve ter pelo menos 4 caracteres")
	}
	salt := randomHexID(16)
	digest := hashSaltedPIN(salt, trimmed)
	return g.setInternalSetting("_auth_pin_hash", salt+"$"+digest)
}

// VerifyAuthPIN verifies candidate against the stored salted PIN hash in constant time.
func (g *GatewayDB) VerifyAuthPIN(candidate string) bool {
	raw, err := g.getInternalSetting("_auth_pin_hash")
	if err != nil || strings.TrimSpace(raw) == "" {
		return false
	}
	salt, expectedHex, ok := strings.Cut(strings.TrimSpace(raw), "$")
	if !ok || salt == "" || expectedHex == "" {
		return false
	}
	actualHex := hashSaltedPIN(salt, strings.TrimSpace(candidate))
	return subtle.ConstantTimeCompare([]byte(actualHex), []byte(expectedHex)) == 1
}

// SaveWebAuthnCredential persists a newly registered biometric passkey credential.
func (g *GatewayDB) SaveWebAuthnCredential(credID, pubX, pubY, label string, signCount uint32) (*WebAuthnCredential, error) {
	id := "cred-" + randomHexID(6)
	cleanLabel := strings.TrimSpace(label)
	if cleanLabel == "" {
		cleanLabel = "Biometria / Passkey"
	}
	now := utcNow()

	g.mu.Lock()
	_, err := g.sql.Exec(
		`INSERT INTO webauthn_credentials (id, credential_id, public_key_x, public_key_y, sign_count, label, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?)
		 ON CONFLICT(credential_id) DO UPDATE SET public_key_x = excluded.public_key_x, public_key_y = excluded.public_key_y, sign_count = excluded.sign_count, label = excluded.label`,
		id, credID, pubX, pubY, int64(signCount), cleanLabel, now,
	)
	g.mu.Unlock()
	if err != nil {
		return nil, err
	}
	return g.GetWebAuthnCredential(credID)
}

// ListWebAuthnCredentials lists all registered passkey credentials (safe metadata + public keys).
func (g *GatewayDB) ListWebAuthnCredentials() ([]WebAuthnCredential, error) {
	g.mu.Lock()
	defer g.mu.Unlock()

	rows, err := g.sql.Query(
		"SELECT id, credential_id, public_key_x, public_key_y, sign_count, label, created_at FROM webauthn_credentials ORDER BY created_at DESC",
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []WebAuthnCredential{}
	for rows.Next() {
		var c WebAuthnCredential
		var sc int64
		if err := rows.Scan(&c.ID, &c.CredentialID, &c.PublicKeyX, &c.PublicKeyY, &sc, &c.Label, &c.CreatedAt); err != nil {
			return nil, err
		}
		if sc >= 0 {
			c.SignCount = uint32(sc)
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// GetWebAuthnCredential loads a credential by its base64url credential_id.
func (g *GatewayDB) GetWebAuthnCredential(credID string) (*WebAuthnCredential, error) {
	g.mu.Lock()
	defer g.mu.Unlock()

	var c WebAuthnCredential
	var sc int64
	err := g.sql.QueryRow(
		"SELECT id, credential_id, public_key_x, public_key_y, sign_count, label, created_at FROM webauthn_credentials WHERE credential_id = ?",
		credID,
	).Scan(&c.ID, &c.CredentialID, &c.PublicKeyX, &c.PublicKeyY, &sc, &c.Label, &c.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if sc >= 0 {
		c.SignCount = uint32(sc)
	}
	return &c, nil
}

// UpdateWebAuthnSignCount updates the monotonic signature counter after a biometric login.
func (g *GatewayDB) UpdateWebAuthnSignCount(credID string, signCount uint32) error {
	g.mu.Lock()
	defer g.mu.Unlock()
	_, err := g.sql.Exec("UPDATE webauthn_credentials SET sign_count = ? WHERE credential_id = ?", int64(signCount), credID)
	return err
}

// DeleteWebAuthnCredential deletes a registered passkey by internal id.
func (g *GatewayDB) DeleteWebAuthnCredential(id string) error {
	g.mu.Lock()
	defer g.mu.Unlock()
	_, err := g.sql.Exec("DELETE FROM webauthn_credentials WHERE id = ?", id)
	return err
}
