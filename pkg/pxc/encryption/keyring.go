package encryption

import (
	"bytes"
	"crypto/rand"
	"crypto/sha256"
	"encoding/binary"
	"fmt"

	"github.com/google/uuid"
)

const (
	version    = "Keyring file version:2.0"
	keyType    = "AES"
	eof        = "EOF"
	userID     = ""
	keyLen     = 32
	paddingLen = 5
)

func NewKeyring() ([]byte, error) {
	key, err := key()
	if err != nil {
		return nil, fmt.Errorf("failed to generate new key: %v", err)
	}

	sha := sha256.Sum256(key)

	keyring := new(bytes.Buffer)

	keyring.WriteString(version)
	keyring.Write(key)
	keyring.WriteString(eof)
	keyring.Write(sha[:])

	return keyring.Bytes(), nil
}

func key() ([]byte, error) {
	keyID := keyID()
	key := new(bytes.Buffer)

	_ = binary.Write(key, binary.LittleEndian, int64(podSize(len(keyID))))
	_ = binary.Write(key, binary.LittleEndian, int64(len(keyID)))
	_ = binary.Write(key, binary.LittleEndian, int64(len(keyType)))
	_ = binary.Write(key, binary.LittleEndian, int64(len(userID)))
	_ = binary.Write(key, binary.LittleEndian, int64(keyLen))

	key.WriteString(keyID)
	key.WriteString(keyType)
	key.WriteString(userID)

	aes, err := aesKey()
	if err != nil {
		return nil, fmt.Errorf("failed to generate AES key: %v", err)
	}

	key.Write(aes)
	key.Write(make([]byte, paddingLen))

	return key.Bytes(), nil
}

func aesKey() ([]byte, error) {
	aes := make([]byte, keyLen)
	_, err := rand.Read(aes)
	if err != nil {
		return nil, fmt.Errorf("failed to generate random sequence of bytes: %v", err)
	}

	obfuscator := []byte("*305=Ljt0*!@$Hnm(*-9-w;:")
	i := 0
	l := 0
	for i < len(aes) {
		aes[i] ^= obfuscator[l]
		i++
		l = (l + 1) % len(obfuscator)
	}

	return aes, nil
}

func podSize(keyIDLen int) int {
	size := 4*8 + keyIDLen + len(keyType) + len(userID) + 8 + keyLen
	padding := (8 - (size % 8)) % 8
	return size + padding
}

func keyID() string {
	return fmt.Sprintf("INNODBKey-%s-1", uuid.New())
}
