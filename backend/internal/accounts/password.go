package accounts

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"fmt"
	"strings"

	"golang.org/x/crypto/argon2"
)

const (
	passwordMemory      = 64 * 1024 // KiB
	passwordIterations  = 3
	passwordParallelism = 2
	passwordSaltBytes   = 16
	passwordKeyBytes    = 32
	minPasswordBytes    = 12
	maxPasswordBytes    = 1024
)

func passwordPrefix() string {
	return fmt.Sprintf("$argon2id$v=%d$m=%d,t=%d,p=%d$", argon2.Version, passwordMemory, passwordIterations, passwordParallelism)
}

func HashPassword(password string) (string, error) {
	if len(password) < minPasswordBytes || len(password) > maxPasswordBytes {
		return "", ErrPassword
	}
	salt := make([]byte, passwordSaltBytes)
	if _, err := rand.Read(salt); err != nil {
		return "", err
	}
	key := argon2.IDKey([]byte(password), salt, passwordIterations, passwordMemory, passwordParallelism, passwordKeyBytes)
	return passwordPrefix() + base64.RawStdEncoding.EncodeToString(salt) + "$" + base64.RawStdEncoding.EncodeToString(key), nil
}

func VerifyPassword(encoded, password string) bool {
	if len(password) > maxPasswordBytes || !strings.HasPrefix(encoded, passwordPrefix()) {
		return false
	}
	parts := strings.Split(strings.TrimPrefix(encoded, passwordPrefix()), "$")
	if len(parts) != 2 {
		return false
	}
	salt, err := base64.RawStdEncoding.DecodeString(parts[0])
	if err != nil || len(salt) != passwordSaltBytes {
		return false
	}
	expected, err := base64.RawStdEncoding.DecodeString(parts[1])
	if err != nil || len(expected) != passwordKeyBytes {
		return false
	}
	actual := argon2.IDKey([]byte(password), salt, passwordIterations, passwordMemory, passwordParallelism, passwordKeyBytes)
	return subtle.ConstantTimeCompare(actual, expected) == 1
}

// 不存在的账号也进行相同成本的校验，避免由耗时暴露用户名。
func dummyPasswordHash() string {
	return passwordPrefix() + base64.RawStdEncoding.EncodeToString(make([]byte, passwordSaltBytes)) + "$" + base64.RawStdEncoding.EncodeToString(make([]byte, passwordKeyBytes))
}
