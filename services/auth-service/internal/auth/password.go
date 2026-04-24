package auth

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"

	"golang.org/x/crypto/argon2"
)

// Argon2id params — recomendação OWASP 2023 para senhas.
// Memory: 19 MiB, iterations: 2, parallelism: 1, salt: 16 bytes, key: 32 bytes.
const (
	argonTime    uint32 = 2
	argonMemory  uint32 = 19 * 1024
	argonThreads uint8  = 1
	argonKeyLen  uint32 = 32
	saltLen             = 16
)

// HashPassword retorna um string codificado no formato PHC:
// $argon2id$v=19$m=19456,t=2,p=1$<b64(salt)>$<b64(hash)>
func HashPassword(password string) (string, error) {
	salt := make([]byte, saltLen)
	if _, err := rand.Read(salt); err != nil {
		return "", fmt.Errorf("salt: %w", err)
	}

	hash := argon2.IDKey([]byte(password), salt, argonTime, argonMemory, argonThreads, argonKeyLen)

	b64Salt := base64.RawStdEncoding.EncodeToString(salt)
	b64Hash := base64.RawStdEncoding.EncodeToString(hash)

	return fmt.Sprintf("$argon2id$v=%d$m=%d,t=%d,p=%d$%s$%s",
		argon2.Version, argonMemory, argonTime, argonThreads, b64Salt, b64Hash), nil
}

// VerifyPassword comparação em tempo constante.
func VerifyPassword(password, encoded string) (bool, error) {
	parts := strings.Split(encoded, "$")
	if len(parts) != 6 || parts[1] != "argon2id" {
		return false, errors.New("invalid hash format")
	}

	var version int
	if _, err := fmt.Sscanf(parts[2], "v=%d", &version); err != nil {
		return false, fmt.Errorf("version: %w", err)
	}
	if version != argon2.Version {
		return false, errors.New("incompatible argon2 version")
	}

	var m, t uint32
	var p uint8
	if _, err := fmt.Sscanf(parts[3], "m=%d,t=%d,p=%d", &m, &t, &p); err != nil {
		return false, fmt.Errorf("params: %w", err)
	}

	salt, err := base64.RawStdEncoding.DecodeString(parts[4])
	if err != nil {
		return false, fmt.Errorf("salt: %w", err)
	}
	wantHash, err := base64.RawStdEncoding.DecodeString(parts[5])
	if err != nil {
		return false, fmt.Errorf("hash: %w", err)
	}

	gotHash := argon2.IDKey([]byte(password), salt, t, m, p, uint32(len(wantHash)))
	return subtle.ConstantTimeCompare(gotHash, wantHash) == 1, nil
}
