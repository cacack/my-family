package auth

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
	"time"

	"golang.org/x/crypto/argon2"
)

// argon2id parameters: the OWASP Password Storage Cheat Sheet's baseline
// (19 MiB, two passes, one lane). A stored hash records its own parameters,
// so raising them later still verifies older hashes.
const (
	argonMemoryKiB = 19 * 1024
	argonTime      = 2
	argonThreads   = 1
	argonKeyLen    = 32
	argonSaltLen   = 16
)

// maxConcurrentHashes caps the argon2id runs in flight across the process.
// Each holds argonMemoryKiB, and sign-in, setup, invite accept and password
// reset run one for callers who are not signed in, so per-client limits alone
// leave the total unbounded. A run that waits longer than hashSlotWait for a
// slot is refused with ErrBusy without hashing (ADR-007).
const (
	maxConcurrentHashes = 4
	hashSlotWait        = time.Second
)

var errMalformedHash = errors.New("malformed password hash")

// hashSlots holds one token per argon2id run in flight. It, slotWait and
// argon2id are variables only so tests can shrink, shorten and observe them.
var (
	hashSlots = make(chan struct{}, maxConcurrentHashes)
	slotWait  = hashSlotWait
	argon2id  = argon2.IDKey
)

// cappedKey runs argon2id within the process-wide cap, or returns ErrBusy
// when no slot frees up within slotWait.
func cappedKey(password, salt []byte, passes, memory uint32, threads uint8, keyLen uint32) ([]byte, error) {
	timer := time.NewTimer(slotWait)
	defer timer.Stop()
	select {
	case hashSlots <- struct{}{}:
	case <-timer.C:
		return nil, ErrBusy
	}
	defer func() { <-hashSlots }()
	return argon2id(password, salt, passes, memory, threads, keyLen), nil
}

// uncappedKey runs argon2id outside the cap, for the one dummy hash a
// Service makes for itself.
func uncappedKey(password, salt []byte, passes, memory uint32, threads uint8, keyLen uint32) ([]byte, error) {
	return argon2id(password, salt, passes, memory, threads, keyLen), nil
}

// keyFunc derives an argon2id key: cappedKey or uncappedKey.
type keyFunc func(password, salt []byte, passes, memory uint32, threads uint8, keyLen uint32) ([]byte, error)

// b64 is the unpadded standard base64 the PHC string format uses.
var b64 = base64.RawStdEncoding

// HashPassword returns an argon2id hash of password in PHC string form:
// $argon2id$v=19$m=...,t=...,p=...$salt$key. It is ErrBusy when the
// process-wide cap has no slot free.
func HashPassword(password string) (string, error) {
	return hashPassword(password, cappedKey)
}

func hashPassword(password string, derive keyFunc) (string, error) {
	salt := make([]byte, argonSaltLen)
	if _, err := rand.Read(salt); err != nil {
		return "", fmt.Errorf("generate salt: %w", err)
	}
	key, err := derive([]byte(password), salt, argonTime, argonMemoryKiB, argonThreads, argonKeyLen)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("$argon2id$v=%d$m=%d,t=%d,p=%d$%s$%s",
		argon2.Version, argonMemoryKiB, argonTime, argonThreads, b64.EncodeToString(salt), b64.EncodeToString(key)), nil
}

// VerifyPassword reports whether password matches hash, a PHC string from
// HashPassword. A hash that does not parse is an error, never a match, and so
// is ErrBusy when the process-wide cap has no slot free.
func VerifyPassword(password, hash string) (bool, error) {
	parts := strings.Split(hash, "$")
	// "", "argon2id", "v=19", "m=..,t=..,p=..", salt, key
	if len(parts) != 6 || parts[0] != "" || parts[1] != "argon2id" {
		return false, errMalformedHash
	}
	var version int
	if _, err := fmt.Sscanf(parts[2], "v=%d", &version); err != nil || version != argon2.Version {
		return false, errMalformedHash
	}
	var memory, passes uint32
	var threads uint8
	if _, err := fmt.Sscanf(parts[3], "m=%d,t=%d,p=%d", &memory, &passes, &threads); err != nil || passes == 0 || threads == 0 {
		return false, errMalformedHash
	}
	salt, err := b64.DecodeString(parts[4])
	if err != nil {
		return false, errMalformedHash
	}
	want, err := b64.DecodeString(parts[5])
	if err != nil || len(want) == 0 {
		return false, errMalformedHash
	}
	got, err := cappedKey([]byte(password), salt, passes, memory, threads, uint32(len(want))) // #nosec G115 -- len(want) is a decoded key length, far below 2^32.
	if err != nil {
		return false, err
	}
	return subtle.ConstantTimeCompare(got, want) == 1, nil
}
