package auth

import (
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
