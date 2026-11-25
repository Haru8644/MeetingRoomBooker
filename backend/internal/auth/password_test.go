package auth

import (
	"bytes"
	"crypto/rand"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"os"
	"testing"
)

func TestVerifyIdentityPassword(t *testing.T) {
	data, err := os.ReadFile("testdata/identity_vectors.json")
	if err != nil {
		t.Fatal(err)
	}
	var vectors []struct {
		Name     string
		Password string
		Hash     string
	}
	if err := json.Unmarshal(data, &vectors); err != nil || len(vectors) != 5 {
		t.Fatal("expected five .NET Identity test vectors")
	}
	for _, vector := range vectors {
		t.Run(vector.Name, func(t *testing.T) {
			if !verifyPassword(vector.Hash, vector.Password) {
				t.Fatal("Go rejected the .NET Identity password")
			}
			if verifyPassword(vector.Hash, vector.Password+"wrong") {
				t.Fatal("Go accepted an incorrect password")
			}
		})
	}
	payload, err := base64.StdEncoding.DecodeString(vectors[0].Hash)
	if err != nil || len(payload) != 61 {
		t.Fatal("expected a default Identity V3 test vector")
	}
	binary.BigEndian.PutUint32(payload[5:9], 1_000_001)
	for _, invalid := range []string{"", "plaintext", "AQ==", base64.StdEncoding.EncodeToString(payload)} {
		if verifyPassword(invalid, vectors[0].Password) {
			t.Fatal("Go accepted a malformed or unsupported hash")
		}
	}
}

func TestHashPasswordIdentityV3(t *testing.T) {
	password := "  " + rand.Text() + "合成  "
	var priorSalt []byte
	for range 2 {
		encoded, err := hashPassword(password)
		if err != nil {
			t.Fatal("could not hash synthetic password")
		}
		payload, err := base64.StdEncoding.DecodeString(encoded)
		if err != nil || len(payload) != 61 || payload[0] != 1 ||
			binary.BigEndian.Uint32(payload[1:5]) != 2 || binary.BigEndian.Uint32(payload[5:9]) != 100_000 ||
			binary.BigEndian.Uint32(payload[9:13]) != 16 {
			t.Fatal("registration hash does not use the default Identity V3 parameters")
		}
		salt := payload[13:29]
		if bytes.Equal(salt, priorSalt) {
			t.Fatal("consecutive hashes used the same salt")
		}
		priorSalt = salt
		if !verifyPassword(encoded, password) || verifyPassword(encoded, "wrong"+password) {
			t.Fatal("registration hash did not authenticate as expected")
		}
	}
}
