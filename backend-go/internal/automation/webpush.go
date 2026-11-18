package automation

import (
	"bytes"
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/ecdh"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/Phoenix7290/agy-mobile-gateway/backend-go/internal/db"
)

// ErrSubscriptionGone indicates the push service permanently rejected the endpoint (404/410).
var ErrSubscriptionGone = errors.New("push subscription expired")

// VAPIDKeys holds base64url-encoded P-256 keys persisted in GatewayDB settings.
type VAPIDKeys struct {
	PublicKey  string `json:"public"`
	PrivateKey string `json:"private"`
}

// BrowserPushSubscription mirrors window.PushSubscription.toJSON().
type BrowserPushSubscription struct {
	Endpoint string `json:"endpoint"`
	Keys     struct {
		P256dh string `json:"p256dh"`
		Auth   string `json:"auth"`
	} `json:"keys"`
}

func decodeB64Flex(s string) ([]byte, error) {
	s = strings.TrimSpace(s)
	if b, err := base64.RawURLEncoding.DecodeString(s); err == nil {
		return b, nil
	}
	if b, err := base64.URLEncoding.DecodeString(s); err == nil {
		return b, nil
	}
	return base64.StdEncoding.DecodeString(s)
}

// GenerateVAPIDKeys creates a fresh P-256 VAPID keypair encoded as raw unpadded base64url.
func GenerateVAPIDKeys() (VAPIDKeys, error) {
	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return VAPIDKeys{}, err
	}
	var privBytes [32]byte
	priv.D.FillBytes(privBytes[:])
	ecdhPriv, err := ecdh.P256().NewPrivateKey(privBytes[:])
	if err != nil {
		return VAPIDKeys{}, err
	}
	pubBytes := ecdhPriv.PublicKey().Bytes()
	return VAPIDKeys{
		PublicKey:  base64.RawURLEncoding.EncodeToString(pubBytes),
		PrivateKey: base64.RawURLEncoding.EncodeToString(privBytes[:]),
	}, nil
}

// EnsureVAPIDKeys loads or lazily generates and stores VAPID keys in SQLite settings.
func EnsureVAPIDKeys(store *db.GatewayDB) (VAPIDKeys, error) {
	raw, err := store.GetOrSetSettingString("vapid_keys_json", func() (string, error) {
		k, genErr := GenerateVAPIDKeys()
		if genErr != nil {
			return "", genErr
		}
		b, mErr := json.Marshal(k)
		return string(b), mErr
	})
	if err != nil {
		return VAPIDKeys{}, err
	}
	var keys VAPIDKeys
	if err := json.Unmarshal([]byte(raw), &keys); err != nil {
		return VAPIDKeys{}, err
	}
	if keys.PublicKey == "" || keys.PrivateKey == "" {
		return VAPIDKeys{}, fmt.Errorf("chaves VAPID inválidas no banco")
	}
	return keys, nil
}

func hkdfExtract(salt, ikm []byte) []byte {
	if len(salt) == 0 {
		salt = make([]byte, sha256.Size)
	}
	mac := hmac.New(sha256.New, salt)
	_, _ = mac.Write(ikm)
	return mac.Sum(nil)
}

func hkdfExpand(prk, info []byte, length int) []byte {
	var out []byte
	var prev []byte
	counter := byte(1)
	for len(out) < length {
		mac := hmac.New(sha256.New, prk)
		_, _ = mac.Write(prev)
		_, _ = mac.Write(info)
		_, _ = mac.Write([]byte{counter})
		prev = mac.Sum(nil)
		out = append(out, prev...)
		counter++
	}
	return out[:length]
}

// EncryptWebPush encrypts plaintext per RFC 8291 (aes128gcm) for a P-256 browser subscription.
func EncryptWebPush(uaPubBytes, authSecret, plaintext []byte) ([]byte, error) {
	if len(uaPubBytes) != 65 || len(authSecret) < 16 {
		return nil, fmt.Errorf("chaves de assinatura push inválidas")
	}
	uaPub, err := ecdh.P256().NewPublicKey(uaPubBytes)
	if err != nil {
		return nil, err
	}
	asPriv, err := ecdh.P256().GenerateKey(rand.Reader)
	if err != nil {
		return nil, err
	}
	asPubBytes := asPriv.PublicKey().Bytes()

	ecdhSecret, err := asPriv.ECDH(uaPub)
	if err != nil {
		return nil, err
	}

	keyInfo := make([]byte, 0, len("WebPush: info\x00")+len(uaPubBytes)+len(asPubBytes))
	keyInfo = append(keyInfo, []byte("WebPush: info\x00")...)
	keyInfo = append(keyInfo, uaPubBytes...)
	keyInfo = append(keyInfo, asPubBytes...)

	prkKey := hkdfExtract(authSecret, ecdhSecret)
	ikm := hkdfExpand(prkKey, keyInfo, 32)

	salt := make([]byte, 16)
	if _, err := rand.Read(salt); err != nil {
		return nil, err
	}
	prk := hkdfExtract(salt, ikm)
	cek := hkdfExpand(prk, []byte("Content-Encoding: aes128gcm\x00"), 16)
	nonce := hkdfExpand(prk, []byte("Content-Encoding: nonce\x00"), 12)

	block, err := aes.NewCipher(cek)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}

	record := make([]byte, len(plaintext)+1)
	copy(record, plaintext)
	record[len(plaintext)] = 0x02

	ciphertext := gcm.Seal(nil, nonce, record, nil)

	var body bytes.Buffer
	body.Write(salt)
	var rs [4]byte
	binary.BigEndian.PutUint32(rs[:], 4096)
	body.Write(rs[:])
	body.WriteByte(byte(len(asPubBytes)))
	body.Write(asPubBytes)
	body.Write(ciphertext)
	return body.Bytes(), nil
}

func vapidSubject() string {
	if sub := strings.TrimSpace(os.Getenv("VAPID_SUBJECT")); sub != "" {
		return sub
	}
	// Apple APNs rejects @localhost subjects; use a syntactically valid mailto domain.
	return "mailto:alerts@agy-gateway.app"
}

func buildVAPIDAuthHeader(endpointURL string, keys VAPIDKeys, exp time.Time) (string, error) {
	u, err := url.Parse(endpointURL)
	if err != nil || u.Scheme == "" || u.Host == "" {
		return "", fmt.Errorf("endpoint push inválido")
	}
	privBytes, err := decodeB64Flex(keys.PrivateKey)
	if err != nil || len(privBytes) != 32 {
		return "", fmt.Errorf("chave privada VAPID inválida")
	}
	pubBytes, err := decodeB64Flex(keys.PublicKey)
	if err != nil || len(pubBytes) != 65 || pubBytes[0] != 0x04 {
		return "", fmt.Errorf("chave pública VAPID inválida")
	}

	ecdsaKey := &ecdsa.PrivateKey{
		PublicKey: ecdsa.PublicKey{
			Curve: elliptic.P256(),
			X:     new(big.Int).SetBytes(pubBytes[1:33]),
			Y:     new(big.Int).SetBytes(pubBytes[33:65]),
		},
		D: new(big.Int).SetBytes(privBytes),
	}

	hdrJSON, _ := json.Marshal(map[string]string{"typ": "JWT", "alg": "ES256"})
	claimsJSON, _ := json.Marshal(map[string]any{
		"aud": u.Scheme + "://" + u.Host,
		"exp": exp.Unix(),
		"sub": vapidSubject(),
	})
	unsigned := base64.RawURLEncoding.EncodeToString(hdrJSON) + "." + base64.RawURLEncoding.EncodeToString(claimsJSON)

	digest := sha256.Sum256([]byte(unsigned))
	r, s, err := ecdsa.Sign(rand.Reader, ecdsaKey, digest[:])
	if err != nil {
		return "", err
	}
	var sig [64]byte
	r.FillBytes(sig[0:32])
	s.FillBytes(sig[32:64])

	jwt := unsigned + "." + base64.RawURLEncoding.EncodeToString(sig[:])
	return fmt.Sprintf("vapid t=%s, k=%s", jwt, keys.PublicKey), nil
}

// SendWebPush encrypts and posts one Web Push notification to a browser endpoint.
func SendWebPush(ctx context.Context, client *http.Client, sub BrowserPushSubscription, keys VAPIDKeys, payload []byte) error {
	uaPub, err := decodeB64Flex(sub.Keys.P256dh)
	if err != nil {
		return fmt.Errorf("p256dh inválido: %w", err)
	}
	authSecret, err := decodeB64Flex(sub.Keys.Auth)
	if err != nil {
		return fmt.Errorf("auth secret inválido: %w", err)
	}
	encrypted, err := EncryptWebPush(uaPub, authSecret, payload)
	if err != nil {
		return err
	}
	authHeader, err := buildVAPIDAuthHeader(sub.Endpoint, keys, time.Now().Add(12*time.Hour))
	if err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, sub.Endpoint, bytes.NewReader(encrypted))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/octet-stream")
	req.Header.Set("Content-Encoding", "aes128gcm")
	req.Header.Set("TTL", "86400")
	req.Header.Set("Urgency", "high")
	req.Header.Set("Authorization", authHeader)

	if client == nil {
		client = &http.Client{Timeout: 8 * time.Second}
	}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	bodyBytes, _ := io.ReadAll(io.LimitReader(resp.Body, 512))

	if resp.StatusCode == http.StatusNotFound || resp.StatusCode == http.StatusGone {
		return ErrSubscriptionGone
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		detail := strings.TrimSpace(string(bodyBytes))
		if detail != "" {
			return fmt.Errorf("push HTTP %d: %s", resp.StatusCode, detail)
		}
		return fmt.Errorf("push HTTP %d", resp.StatusCode)
	}
	return nil
}
