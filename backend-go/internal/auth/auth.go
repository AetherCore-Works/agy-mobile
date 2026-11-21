package auth

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/Phoenix7290/agy-mobile-gateway/backend-go/internal/config"
	"github.com/Phoenix7290/agy-mobile-gateway/backend-go/internal/db"
)

const (
	sessionCookieName = "agy_session"
	sessionTTL        = 30 * 24 * time.Hour
	challengeTTL      = 5 * time.Minute
)

// Manager handles PIN/Token auth, signed HttpOnly cookies, and pure-Go ES256 WebAuthn passkeys.
type Manager struct {
	cfg        config.GatewaySettings
	store      *db.GatewayDB
	secretOnce sync.Once
	secretKey  []byte

	chalMu     sync.Mutex
	challenges map[string]challengeEntry
}

type challengeEntry struct {
	kind      string // "register" | "login"
	expiresAt time.Time
}

// NewManager creates an auth Manager wired to config and SQLite store.
func NewManager(cfg config.GatewaySettings, store *db.GatewayDB) *Manager {
	return &Manager{
		cfg:        cfg,
		store:      store,
		challenges: map[string]challengeEntry{},
	}
}

func (m *Manager) signingKey() []byte {
	m.secretOnce.Do(func() {
		dbKey, err := m.store.GetOrCreateAuthSecret()
		if err != nil || len(dbKey) == 0 {
			dbKey = make([]byte, 32)
			_, _ = rand.Read(dbKey)
		}
		h := hmac.New(sha256.New, dbKey)
		h.Write([]byte("agy-session-v1|" + m.cfg.AuthToken))
		m.secretKey = h.Sum(nil)
	})
	return m.secretKey
}

// IsEnabled returns true when GATEWAY_AUTH_TOKEN is configured or a UI PIN exists in SQLite.
func (m *Manager) IsEnabled() bool {
	return strings.TrimSpace(m.cfg.AuthToken) != "" || m.store.HasAuthPIN()
}

// VerifySecret checks candidate against GATEWAY_AUTH_TOKEN or the persisted UI PIN hash.
func (m *Manager) VerifySecret(candidate string) bool {
	clean := strings.TrimSpace(candidate)
	if clean == "" {
		return false
	}
	if expected := strings.TrimSpace(m.cfg.AuthToken); expected != "" {
		if subtle.ConstantTimeCompare([]byte(clean), []byte(expected)) == 1 {
			return true
		}
	}
	return m.store.VerifyAuthPIN(clean)
}

func (m *Manager) signSessionValue(expUnix int64) string {
	nonce := make([]byte, 12)
	_, _ = rand.Read(nonce)
	payload := fmt.Sprintf("%d.%s", expUnix, hex.EncodeToString(nonce))
	mac := hmac.New(sha256.New, m.signingKey())
	mac.Write([]byte(payload))
	sig := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
	return payload + "." + sig
}

func (m *Manager) verifySessionValue(raw string) bool {
	parts := strings.Split(strings.TrimSpace(raw), ".")
	if len(parts) != 3 {
		return false
	}
	var expUnix int64
	if _, err := fmt.Sscan(parts[0], &expUnix); err != nil {
		return false
	}
	if time.Now().Unix() > expUnix {
		return false
	}
	payload := parts[0] + "." + parts[1]
	mac := hmac.New(sha256.New, m.signingKey())
	mac.Write([]byte(payload))
	expectedSig := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
	return subtle.ConstantTimeCompare([]byte(parts[2]), []byte(expectedSig)) == 1
}

// IsAuthenticated checks whether the HTTP request carries a valid HttpOnly session cookie or Bearer token.
func (m *Manager) IsAuthenticated(r *http.Request) bool {
	if !m.IsEnabled() {
		return true
	}
	if expected := strings.TrimSpace(m.cfg.AuthToken); expected != "" {
		authHdr := strings.TrimSpace(r.Header.Get("Authorization"))
		const prefix = "Bearer "
		if len(authHdr) > len(prefix) && strings.EqualFold(authHdr[:len(prefix)], prefix) {
			tok := strings.TrimSpace(authHdr[len(prefix):])
			if subtle.ConstantTimeCompare([]byte(tok), []byte(expected)) == 1 {
				return true
			}
		}
	}
	cookie, err := r.Cookie(sessionCookieName)
	if err != nil {
		return false
	}
	return m.verifySessionValue(cookie.Value)
}

func isHTTPSRequest(r *http.Request) bool {
	if r.TLS != nil {
		return true
	}
	return strings.EqualFold(strings.TrimSpace(r.Header.Get("X-Forwarded-Proto")), "https")
}

// SetSessionCookie issues a signed HttpOnly session cookie on w.
func (m *Manager) SetSessionCookie(w http.ResponseWriter, r *http.Request) {
	exp := time.Now().Add(sessionTTL)
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    m.signSessionValue(exp.Unix()),
		Path:     "/",
		Expires:  exp,
		MaxAge:   int(sessionTTL.Seconds()),
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   isHTTPSRequest(r),
	})
}

// ClearSessionCookie revokes the session cookie immediately.
func (m *Manager) ClearSessionCookie(w http.ResponseWriter, r *http.Request) {
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   isHTTPSRequest(r),
	})
}

func isPublicPath(path string) bool {
	if path == "/" ||
		strings.HasPrefix(path, "/assets/") ||
		strings.HasPrefix(path, "/static/") ||
		strings.HasPrefix(path, "/s/") ||
		path == "/manifest.json" ||
		path == "/sw.js" ||
		path == "/api/health" ||
		path == "/api/auth/status" ||
		path == "/api/auth/login" ||
		path == "/api/auth/logout" ||
		path == "/api/auth/webauthn/login/options" ||
		path == "/api/auth/webauthn/login/verify" ||
		path == "/api/webhook/prompt" ||
		path == "/api/bot/whatsapp/webhook" ||
		path == "/api/bot/discord/dm" ||
		path == "/api/bot/telegram/webhook" {
		return true
	}
	return false
}

// Middleware protects private API and WebSocket routes when authentication is enabled.
func (m *Manager) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !m.IsEnabled() || isPublicPath(r.URL.Path) || m.IsAuthenticated(r) {
			next.ServeHTTP(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"detail":"Autenticação necessária"}`))
	})
}

// IssueChallenge creates a short-lived WebAuthn base64url challenge.
func (m *Manager) IssueChallenge(kind string) string {
	b := make([]byte, 32)
	_, _ = rand.Read(b)
	chal := base64.RawURLEncoding.EncodeToString(b)
	now := time.Now()

	m.chalMu.Lock()
	for k, v := range m.challenges {
		if now.After(v.expiresAt) {
			delete(m.challenges, k)
		}
	}
	m.challenges[chal] = challengeEntry{kind: kind, expiresAt: now.Add(challengeTTL)}
	m.chalMu.Unlock()
	return chal
}

func (m *Manager) consumeChallenge(chal, expectedKind string) bool {
	now := time.Now()
	m.chalMu.Lock()
	defer m.chalMu.Unlock()
	entry, ok := m.challenges[chal]
	if ok {
		delete(m.challenges, chal)
	}
	return ok && entry.kind == expectedKind && now.Before(entry.expiresAt)
}

type clientDataJSON struct {
	Type      string `json:"type"`
	Challenge string `json:"challenge"`
	Origin    string `json:"origin"`
}

// VerifyAndStoreRegistration verifies an ES256 WebAuthn registration response and saves the credential.
func (m *Manager) VerifyAndStoreRegistration(clientDataB64, attestationObjB64, label string) (*db.WebAuthnCredential, error) {
	cdBytes, err := base64.RawURLEncoding.DecodeString(strings.TrimRight(clientDataB64, "="))
	if err != nil {
		return nil, fmt.Errorf("clientDataJSON inválido: %w", err)
	}
	var cd clientDataJSON
	if err := json.Unmarshal(cdBytes, &cd); err != nil {
		return nil, fmt.Errorf("JSON clientData inválido: %w", err)
	}
	if cd.Type != "webauthn.create" || !m.consumeChallenge(cd.Challenge, "register") {
		return nil, errors.New("challenge de registro WebAuthn inválido ou expirado")
	}

	attBytes, err := base64.RawURLEncoding.DecodeString(strings.TrimRight(attestationObjB64, "="))
	if err != nil {
		return nil, fmt.Errorf("attestationObject inválido: %w", err)
	}

	authData, err := extractAuthDataFromAttestation(attBytes)
	if err != nil {
		return nil, err
	}
	if len(authData) < 37 || (authData[32]&0x40) == 0 {
		return nil, errors.New("attestedCredentialData ausente")
	}

	signCount := binary.BigEndian.Uint32(authData[33:37])
	pos := 37 + 16 // skip rpIdHash(32)+flags(1)+signCount(4)+aaguid(16)
	if len(authData) < pos+2 {
		return nil, errors.New("authData truncado no comprimento da credencial")
	}
	credIDLen := int(binary.BigEndian.Uint16(authData[pos : pos+2]))
	pos += 2
	if len(authData) < pos+credIDLen {
		return nil, errors.New("authData truncado no credentialId")
	}
	credIDBytes := authData[pos : pos+credIDLen]
	coseBytes := authData[pos+credIDLen:]

	xHex, yHex, err := parseCOSEEC2P256(coseBytes)
	if err != nil {
		return nil, err
	}
	credID := base64.RawURLEncoding.EncodeToString(credIDBytes)
	return m.store.SaveWebAuthnCredential(credID, xHex, yHex, label, signCount)
}

// VerifyAssertion verifies an ES256 WebAuthn assertion against stored credentials.
func (m *Manager) VerifyAssertion(credIDB64, clientDataB64, authDataB64, sigB64 string) error {
	cdBytes, err := base64.RawURLEncoding.DecodeString(strings.TrimRight(clientDataB64, "="))
	if err != nil {
		return fmt.Errorf("clientDataJSON inválido: %w", err)
	}
	var cd clientDataJSON
	if err := json.Unmarshal(cdBytes, &cd); err != nil {
		return fmt.Errorf("JSON clientData inválido: %w", err)
	}
	if cd.Type != "webauthn.get" || !m.consumeChallenge(cd.Challenge, "login") {
		return errors.New("challenge de autenticação WebAuthn inválido ou expirado")
	}

	cred, err := m.store.GetWebAuthnCredential(strings.TrimRight(credIDB64, "="))
	if err != nil || cred == nil {
		return errors.New("credencial biométrica não reconhecida")
	}

	authData, err := base64.RawURLEncoding.DecodeString(strings.TrimRight(authDataB64, "="))
	if err != nil || len(authData) < 37 {
		return errors.New("authenticatorData inválido")
	}
	if (authData[32] & 0x01) == 0 {
		return errors.New("presença de usuário não confirmada")
	}

	sigBytes, err := base64.RawURLEncoding.DecodeString(strings.TrimRight(sigB64, "="))
	if err != nil {
		return errors.New("assinatura inválida")
	}

	xBytes, err1 := hex.DecodeString(cred.PublicKeyX)
	yBytes, err2 := hex.DecodeString(cred.PublicKeyY)
	if err1 != nil || err2 != nil {
		return errors.New("chave pública armazenada inválida")
	}
	pub := &ecdsa.PublicKey{
		Curve: elliptic.P256(),
		X:     new(big.Int).SetBytes(xBytes),
		Y:     new(big.Int).SetBytes(yBytes),
	}

	cdHash := sha256.Sum256(cdBytes)
	signed := make([]byte, 0, len(authData)+len(cdHash))
	signed = append(signed, authData...)
	signed = append(signed, cdHash[:]...)
	digest := sha256.Sum256(signed)

	if !ecdsa.VerifyASN1(pub, digest[:], sigBytes) {
		return errors.New("falha na verificação da assinatura biométrica")
	}

	newCount := binary.BigEndian.Uint32(authData[33:37])
	if newCount > cred.SignCount {
		_ = m.store.UpdateWebAuthnSignCount(cred.CredentialID, newCount)
	}
	return nil
}

// --- Minimal deterministic CBOR helpers for WebAuthn attestation & COSE P-256 keys ---

type cborReader struct {
	b []byte
	i int
}

func (r *cborReader) readHead() (major byte, val uint64, err error) {
	if r.i >= len(r.b) {
		return 0, 0, errors.New("unexpected end of CBOR")
	}
	ib := r.b[r.i]
	r.i++
	major = ib >> 5
	ai := ib & 0x1f
	switch {
	case ai < 24:
		return major, uint64(ai), nil
	case ai == 24:
		if r.i+1 > len(r.b) {
			return 0, 0, errors.New("truncated CBOR uint8")
		}
		v := uint64(r.b[r.i])
		r.i++
		return major, v, nil
	case ai == 25:
		if r.i+2 > len(r.b) {
			return 0, 0, errors.New("truncated CBOR uint16")
		}
		v := uint64(binary.BigEndian.Uint16(r.b[r.i : r.i+2]))
		r.i += 2
		return major, v, nil
	case ai == 26:
		if r.i+4 > len(r.b) {
			return 0, 0, errors.New("truncated CBOR uint32")
		}
		v := uint64(binary.BigEndian.Uint32(r.b[r.i : r.i+4]))
		r.i += 4
		return major, v, nil
	case ai == 27:
		if r.i+8 > len(r.b) {
			return 0, 0, errors.New("truncated CBOR uint64")
		}
		v := binary.BigEndian.Uint64(r.b[r.i : r.i+8])
		r.i += 8
		return major, v, nil
	default:
		return 0, 0, errors.New("unsupported CBOR additional info")
	}
}

func (r *cborReader) readAny() (any, error) {
	major, val, err := r.readHead()
	if err != nil {
		return nil, err
	}
	switch major {
	case 0:
		return int64(val), nil
	case 1:
		return -1 - int64(val), nil
	case 2:
		if r.i+int(val) > len(r.b) {
			return nil, errors.New("truncated CBOR bytes")
		}
		out := make([]byte, int(val))
		copy(out, r.b[r.i:r.i+int(val)])
		r.i += int(val)
		return out, nil
	case 3:
		if r.i+int(val) > len(r.b) {
			return nil, errors.New("truncated CBOR text")
		}
		out := string(r.b[r.i : r.i+int(val)])
		r.i += int(val)
		return out, nil
	case 4:
		arr := make([]any, 0, int(val))
		for k := 0; k < int(val); k++ {
			elem, err := r.readAny()
			if err != nil {
				return nil, err
			}
			arr = append(arr, elem)
		}
		return arr, nil
	case 5:
		mp := make(map[any]any, int(val))
		for k := 0; k < int(val); k++ {
			mk, err := r.readAny()
			if err != nil {
				return nil, err
			}
			mv, err := r.readAny()
			if err != nil {
				return nil, err
			}
			mp[mk] = mv
		}
		return mp, nil
	case 6:
		return r.readAny()
	case 7:
		return val, nil
	default:
		return nil, errors.New("unknown CBOR major type")
	}
}

func extractAuthDataFromAttestation(raw []byte) ([]byte, error) {
	rd := &cborReader{b: raw}
	v, err := rd.readAny()
	if err != nil {
		return nil, err
	}
	mp, ok := v.(map[any]any)
	if !ok {
		return nil, errors.New("attestationObject não é um mapa CBOR")
	}
	ad, ok := mp["authData"].([]byte)
	if !ok {
		return nil, errors.New("authData ausente no attestationObject")
	}
	return ad, nil
}

func parseCOSEEC2P256(raw []byte) (xHex, yHex string, err error) {
	rd := &cborReader{b: raw}
	v, err := rd.readAny()
	if err != nil {
		return "", "", err
	}
	mp, ok := v.(map[any]any)
	if !ok {
		return "", "", errors.New("chave pública COSE inválida")
	}
	kty, _ := mp[int64(1)].(int64)
	alg, _ := mp[int64(3)].(int64)
	crv, _ := mp[int64(-1)].(int64)
	if kty != 2 || alg != -7 || crv != 1 {
		return "", "", fmt.Errorf("algoritmo WebAuthn não suportado (use ES256/P-256; kty=%d alg=%d crv=%d)", kty, alg, crv)
	}
	xBytes, ok1 := mp[int64(-2)].([]byte)
	yBytes, ok2 := mp[int64(-3)].([]byte)
	if !ok1 || !ok2 || len(xBytes) != 32 || len(yBytes) != 32 {
		return "", "", errors.New("coordenadas P-256 inválidas na chave COSE")
	}
	return hex.EncodeToString(xBytes), hex.EncodeToString(yBytes), nil
}
