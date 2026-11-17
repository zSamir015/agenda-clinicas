package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	"github.com/pquerna/otp"
	"github.com/pquerna/otp/totp"
)

// DefaultRecoveryCodeCount is the number of single-use backup codes generated per user.
const DefaultRecoveryCodeCount = 10

// TOTPSetup holds a newly generated TOTP secret and its otpauth:// provisioning URI.
type TOTPSetup struct {
	Secret string
	URI    string
}

// GenerateTOTP creates a new TOTP secret for accountName under issuer.
func GenerateTOTP(issuer, accountName string) (TOTPSetup, error) {
	key, err := totp.Generate(totp.GenerateOpts{
		Issuer:      issuer,
		AccountName: accountName,
		Period:      30,
		Digits:      otp.DigitsSix,
		Algorithm:   otp.AlgorithmSHA1,
	})
	if err != nil {
		return TOTPSetup{}, fmt.Errorf("generate totp key: %w", err)
	}
	return TOTPSetup{
		Secret: key.Secret(),
		URI:    key.URL(),
	}, nil
}

// VerifyTOTP validates a 6-digit TOTP code at timestamp now (with +-1 step skew).
func VerifyTOTP(secret, code string, now time.Time) bool {
	code = strings.TrimSpace(code)
	if secret == "" || code == "" {
		return false
	}
	ok, err := totp.ValidateCustom(code, secret, now, totp.ValidateOpts{
		Period:    30,
		Skew:      1,
		Digits:    otp.DigitsSix,
		Algorithm: otp.AlgorithmSHA1,
	})
	return err == nil && ok
}

// GenerateRecoveryCodes generates count single-use recovery codes and returns
// both the plaintext codes (to show once to the user) and their SHA-256 hashes.
func GenerateRecoveryCodes(count int) ([]string, []string, error) {
	if count <= 0 {
		count = DefaultRecoveryCodeCount
	}
	plain := make([]string, 0, count)
	hashes := make([]string, 0, count)
	buf := make([]byte, 5)
	for range count {
		if _, err := rand.Read(buf); err != nil {
			return nil, nil, fmt.Errorf("generate recovery code: %w", err)
		}
		raw := strings.ToUpper(hex.EncodeToString(buf))
		formatted := raw[:5] + "-" + raw[5:]
		plain = append(plain, formatted)
		hashes = append(hashes, hashRecoveryCode(formatted))
	}
	return plain, hashes, nil
}

// ConsumeRecoveryCode checks code against stored SHA-256 hashes in constant time
// and returns the remaining hashes with the matched entry removed.
func ConsumeRecoveryCode(hashes []string, code string) ([]string, bool) {
	target := hashRecoveryCode(code)
	matchIdx := -1
	for i, h := range hashes {
		if subtle.ConstantTimeCompare([]byte(h), []byte(target)) == 1 {
			matchIdx = i
		}
	}
	if matchIdx < 0 {
		return hashes, false
	}
	remaining := make([]string, 0, len(hashes)-1)
	remaining = append(remaining, hashes[:matchIdx]...)
	remaining = append(remaining, hashes[matchIdx+1:]...)
	return remaining, true
}

func hashRecoveryCode(code string) string {
	norm := strings.ToUpper(strings.ReplaceAll(strings.TrimSpace(code), "-", ""))
	sum := sha256.Sum256([]byte(norm))
	return hex.EncodeToString(sum[:])
}
