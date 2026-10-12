package relay

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"

	"golang.org/x/crypto/argon2"
)

// The parameters RFC 9106 lists as the second recommended option, which fits
// a small server: 64 MiB per hash, so a login burst is bounded by AuthPerIP.
const (
	argonMemory  = 64 * 1024
	argonTime    = 3
	argonThreads = 4
	argonSaltLen = 16
	argonKeyLen  = 32
)

type argonParams struct {
	memory  uint32
	time    uint32
	threads uint8
	salt    []byte
	key     []byte
}

var errBadHash = errors.New("password hash must be $argon2id$v=19$m=<kib>,t=<iterations>,p=<threads>$<salt>$<hash>")

// HashPassword returns pw as an encoded argon2id hash:
// $argon2id$v=19$m=65536,t=3,p=4$<salt>$<hash>, both parts unpadded base64.
func HashPassword(pw string) (string, error) {
	salt := make([]byte, argonSaltLen)
	if _, err := rand.Read(salt); err != nil {
		return "", fmt.Errorf("generate salt: %w", err)
	}
	key := argon2.IDKey([]byte(pw), salt, argonTime, argonMemory, argonThreads, argonKeyLen)
	return fmt.Sprintf("$argon2id$v=%d$m=%d,t=%d,p=%d$%s$%s",
		argon2.Version, argonMemory, argonTime, argonThreads,
		base64.RawStdEncoding.EncodeToString(salt),
		base64.RawStdEncoding.EncodeToString(key),
	), nil
}

// VerifyPassword reports whether pw matches encoded, hashing with the
// parameters read from encoded so old hashes keep working if the defaults move.
func VerifyPassword(encoded, pw string) (bool, error) {
	p, err := parseArgon(encoded)
	if err != nil {
		return false, err
	}
	key := argon2.IDKey([]byte(pw), p.salt, p.time, p.memory, p.threads, uint32(len(p.key)))
	return subtle.ConstantTimeCompare(key, p.key) == 1, nil
}

// ParsePasswordHash checks that encoded is a well-formed argon2id hash.
func ParsePasswordHash(encoded string) error {
	_, err := parseArgon(encoded)
	return err
}

func parseArgon(encoded string) (argonParams, error) {
	parts := strings.Split(encoded, "$")
	if len(parts) != 6 || parts[0] != "" || parts[1] != "argon2id" {
		return argonParams{}, errBadHash
	}
	var version int
	if _, err := fmt.Sscanf(parts[2], "v=%d", &version); err != nil {
		return argonParams{}, errBadHash
	}
	if version != argon2.Version {
		return argonParams{}, fmt.Errorf("argon2id version %d is not supported", version)
	}
	var p argonParams
	if _, err := fmt.Sscanf(parts[3], "m=%d,t=%d,p=%d", &p.memory, &p.time, &p.threads); err != nil {
		return argonParams{}, errBadHash
	}
	if p.memory == 0 || p.time == 0 || p.threads == 0 {
		return argonParams{}, errBadHash
	}
	var err error
	if p.salt, err = base64.RawStdEncoding.DecodeString(parts[4]); err != nil || len(p.salt) == 0 {
		return argonParams{}, errBadHash
	}
	if p.key, err = base64.RawStdEncoding.DecodeString(parts[5]); err != nil || len(p.key) == 0 {
		return argonParams{}, errBadHash
	}
	return p, nil
}
