package auth

import (
	"crypto/pbkdf2"
	"crypto/rand"
	"crypto/sha1"
	"crypto/sha256"
	"crypto/sha512"
	"crypto/subtle"
	"encoding/base64"
	"encoding/binary"
	"hash"
)

const identityV3HeaderSize = 13

func hashPassword(password string) (string, error) {
	const (
		iterations = 100_000
		saltSize   = 16
		keySize    = 32
	)
	payload := make([]byte, identityV3HeaderSize+saltSize+keySize)
	payload[0] = 1
	binary.BigEndian.PutUint32(payload[1:5], 2) // HMAC-SHA512
	binary.BigEndian.PutUint32(payload[5:9], iterations)
	binary.BigEndian.PutUint32(payload[9:13], saltSize)
	salt := payload[identityV3HeaderSize : identityV3HeaderSize+saltSize]
	rand.Read(salt)
	key, err := pbkdf2.Key(sha512.New, password, salt, iterations, keySize)
	if err != nil {
		return "", err
	}
	copy(payload[identityV3HeaderSize+saltSize:], key)
	return base64.StdEncoding.EncodeToString(payload), nil
}

func verifyPassword(encoded, password string) bool {
	// Identity stores a format marker followed by the PBKDF2 parameters and subkey.
	if len(encoded) > 256 {
		return false
	}
	payload, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil || len(payload) == 0 {
		return false
	}
	if payload[0] == 0 && len(payload) == 49 {
		return matchesSubkey(sha1.New, password, payload[1:17], payload[17:], 1000)
	}
	if payload[0] != 1 || len(payload) < identityV3HeaderSize+32 {
		return false
	}
	iterations := binary.BigEndian.Uint32(payload[5:9])
	saltLength := binary.BigEndian.Uint32(payload[9:13])
	if iterations == 0 || iterations > 1_000_000 || saltLength < 16 || saltLength > uint32(len(payload)-identityV3HeaderSize-16) {
		return false
	}
	var algorithm func() hash.Hash
	switch binary.BigEndian.Uint32(payload[1:5]) {
	case 0:
		algorithm = sha1.New
	case 1:
		algorithm = sha256.New
	case 2:
		algorithm = sha512.New
	default:
		return false
	}
	subkeyOffset := identityV3HeaderSize + int(saltLength)
	return matchesSubkey(algorithm, password, payload[identityV3HeaderSize:subkeyOffset], payload[subkeyOffset:], int(iterations))
}

func matchesSubkey(algorithm func() hash.Hash, password string, salt, expected []byte, iterations int) bool {
	actual, err := pbkdf2.Key(algorithm, password, salt, iterations, len(expected))
	return err == nil && subtle.ConstantTimeCompare(actual, expected) == 1
}
