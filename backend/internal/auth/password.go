package auth

import (
	"crypto/pbkdf2"
	"crypto/sha1"
	"crypto/sha256"
	"crypto/sha512"
	"crypto/subtle"
	"encoding/base64"
	"encoding/binary"
	"hash"
)

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
	const headerSize = 13
	if payload[0] != 1 || len(payload) < headerSize+32 {
		return false
	}
	iterations := binary.BigEndian.Uint32(payload[5:9])
	saltLength := binary.BigEndian.Uint32(payload[9:13])
	if iterations == 0 || iterations > 1_000_000 || saltLength < 16 || saltLength > uint32(len(payload)-headerSize-16) {
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
	subkeyOffset := headerSize + int(saltLength)
	return matchesSubkey(algorithm, password, payload[headerSize:subkeyOffset], payload[subkeyOffset:], int(iterations))
}

func matchesSubkey(algorithm func() hash.Hash, password string, salt, expected []byte, iterations int) bool {
	actual, err := pbkdf2.Key(algorithm, password, salt, iterations, len(expected))
	return err == nil && subtle.ConstantTimeCompare(actual, expected) == 1
}
