package skillmarket

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"infinite-canvas/backend/internal/skillhub"
)

const signingKeyFile = "skillhub-signing.key"

// Signer holds the platform's Ed25519 release key. Kernels trust its public
// key through BEEFTV_SKILLHUB_TRUSTED_KEYS and refuse unsigned releases.
type Signer struct {
	keyID   string
	private ed25519.PrivateKey
}

// LoadSigner reads <secretsDir>/skillhub-signing.key (base64 seed), creating
// it with mode 0600 on first start.
func LoadSigner(secretsDir string) (*Signer, error) {
	path := filepath.Join(secretsDir, signingKeyFile)
	raw, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		seed := make([]byte, ed25519.SeedSize)
		if _, err := rand.Read(seed); err != nil {
			return nil, err
		}
		encoded := base64.StdEncoding.EncodeToString(seed)
		if err := os.WriteFile(path, []byte(encoded+"\n"), 0o600); err != nil {
			return nil, fmt.Errorf("写入技能签名密钥失败：%w", err)
		}
		raw = []byte(encoded)
	} else if err != nil {
		return nil, fmt.Errorf("读取技能签名密钥失败：%w", err)
	}
	seed, err := base64.StdEncoding.DecodeString(strings.TrimSpace(string(raw)))
	if err != nil || len(seed) != ed25519.SeedSize {
		return nil, errors.New("技能签名密钥损坏")
	}
	private := ed25519.NewKeyFromSeed(seed)
	sum := sha256.Sum256(private.Public().(ed25519.PublicKey))
	return &Signer{keyID: "teamserver-" + hex.EncodeToString(sum[:4]), private: private}, nil
}

func (s *Signer) KeyID() string { return s.keyID }

// TrustedKeysJSON is the value for BEEFTV_SKILLHUB_TRUSTED_KEYS.
func (s *Signer) TrustedKeysJSON() string {
	public := s.private.Public().(ed25519.PublicKey)
	encoded, _ := json.Marshal(map[string]string{s.keyID: base64.StdEncoding.EncodeToString(public)})
	return string(encoded)
}

// Sign produces the attestation over the release JSON without attestation,
// in the exact format skillhub.VerifyRelease checks.
func (s *Signer) Sign(release skillhub.Release) ([]byte, *skillhub.Attestation, error) {
	release.Attestation = nil
	payload, err := json.Marshal(release)
	if err != nil {
		return nil, nil, err
	}
	signature := ed25519.Sign(s.private, payload)
	return payload, &skillhub.Attestation{Algorithm: "Ed25519", KeyID: s.keyID,
		Payload: base64.StdEncoding.EncodeToString(payload), Signature: base64.StdEncoding.EncodeToString(signature)}, nil
}
