package observability

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

const (
	encryptedBackupFormat     = "cliproxy-pro-encrypted-backup"
	encryptedBackupVersion    = 2
	encryptedBackupLegacyV1   = 1
	encryptedBackupIterations = 210000
	encryptedBackupKeyLength  = 32
)

type encryptedBackupKDF struct {
	Name       string `json:"name"`
	Iterations int    `json:"iterations"`
	Salt       string `json:"salt"`
}

type encryptedBackupCipher struct {
	Name  string `json:"name"`
	Nonce string `json:"nonce"`
}

type encryptedBackupEnvelope struct {
	Format        string                `json:"format"`
	Version       int                   `json:"version"`
	KDF           encryptedBackupKDF    `json:"kdf"`
	Cipher        encryptedBackupCipher `json:"cipher"`
	Ciphertext    string                `json:"ciphertext"`
	CreatedAtMS   int64                 `json:"createdAtMs"`
	SecretClasses []string              `json:"secretClasses,omitempty"`
}

func encryptedBackupAuthenticatedData(envelope encryptedBackupEnvelope) ([]byte, error) {
	return json.Marshal(struct {
		Format        string   `json:"format"`
		Version       int      `json:"version"`
		CreatedAtMS   int64    `json:"createdAtMs"`
		SecretClasses []string `json:"secretClasses,omitempty"`
	}{
		Format: envelope.Format, Version: envelope.Version, CreatedAtMS: envelope.CreatedAtMS,
		SecretClasses: envelope.SecretClasses,
	})
}

func derivePBKDF2SHA256(password, salt []byte, iterations, keyLength int) ([]byte, error) {
	if len(password) == 0 || len(salt) < 16 || iterations < 100000 || iterations > 2000000 || keyLength <= 0 || keyLength > sha256.Size*4 {
		return nil, fmt.Errorf("invalid encrypted backup KDF parameters")
	}
	blocks := (keyLength + sha256.Size - 1) / sha256.Size
	derived := make([]byte, 0, blocks*sha256.Size)
	for block := 1; block <= blocks; block++ {
		mac := hmac.New(sha256.New, password)
		_, _ = mac.Write(salt)
		var counter [4]byte
		binary.BigEndian.PutUint32(counter[:], uint32(block))
		_, _ = mac.Write(counter[:])
		previous := mac.Sum(nil)
		value := append([]byte(nil), previous...)
		for iteration := 1; iteration < iterations; iteration++ {
			mac = hmac.New(sha256.New, password)
			_, _ = mac.Write(previous)
			previous = mac.Sum(nil)
			for index := range value {
				value[index] ^= previous[index]
			}
		}
		derived = append(derived, value...)
	}
	return derived[:keyLength], nil
}

func encryptBackup(data []byte, passphrase string, secretClasses []string) ([]byte, error) {
	return encryptBackupVersion(data, passphrase, secretClasses, encryptedBackupVersion)
}

func encryptBackupVersion(data []byte, passphrase string, secretClasses []string, version int) ([]byte, error) {
	if len([]rune(strings.TrimSpace(passphrase))) < 8 {
		return nil, fmt.Errorf("backup passphrase must contain at least 8 characters")
	}
	if version != encryptedBackupVersion && version != encryptedBackupLegacyV1 {
		return nil, fmt.Errorf("unsupported encrypted backup format")
	}
	salt := make([]byte, 16)
	if _, err := rand.Read(salt); err != nil {
		return nil, err
	}
	key, err := derivePBKDF2SHA256([]byte(passphrase), salt, encryptedBackupIterations, encryptedBackupKeyLength)
	if err != nil {
		return nil, err
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	nonce := make([]byte, aead.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return nil, err
	}
	envelope := encryptedBackupEnvelope{
		Format: encryptedBackupFormat, Version: version,
		KDF:           encryptedBackupKDF{Name: "pbkdf2-sha256", Iterations: encryptedBackupIterations, Salt: base64.StdEncoding.EncodeToString(salt)},
		Cipher:        encryptedBackupCipher{Name: "aes-256-gcm", Nonce: base64.StdEncoding.EncodeToString(nonce)},
		CreatedAtMS:   time.Now().UnixMilli(),
		SecretClasses: append([]string(nil), secretClasses...),
	}
	authenticatedData := []byte(encryptedBackupFormat + ":1")
	if version == encryptedBackupVersion {
		authenticatedData, err = encryptedBackupAuthenticatedData(envelope)
		if err != nil {
			return nil, err
		}
	}
	envelope.Ciphertext = base64.StdEncoding.EncodeToString(aead.Seal(nil, nonce, data, authenticatedData))
	return json.Marshal(envelope)
}

func decryptBackup(data []byte, passphrase string) ([]byte, bool, []string, error) {
	var envelope encryptedBackupEnvelope
	if err := json.Unmarshal(data, &envelope); err != nil || envelope.Format != encryptedBackupFormat {
		return data, false, nil, nil
	}
	if (envelope.Version != encryptedBackupVersion && envelope.Version != encryptedBackupLegacyV1) || envelope.KDF.Name != "pbkdf2-sha256" || envelope.Cipher.Name != "aes-256-gcm" {
		return nil, true, envelope.SecretClasses, fmt.Errorf("unsupported encrypted backup format")
	}
	if strings.TrimSpace(passphrase) == "" {
		return nil, true, envelope.SecretClasses, fmt.Errorf("encrypted backup passphrase is required")
	}
	salt, err := base64.StdEncoding.DecodeString(envelope.KDF.Salt)
	if err != nil {
		return nil, true, envelope.SecretClasses, fmt.Errorf("invalid encrypted backup salt")
	}
	nonce, err := base64.StdEncoding.DecodeString(envelope.Cipher.Nonce)
	if err != nil {
		return nil, true, envelope.SecretClasses, fmt.Errorf("invalid encrypted backup nonce")
	}
	ciphertext, err := base64.StdEncoding.DecodeString(envelope.Ciphertext)
	if err != nil {
		return nil, true, envelope.SecretClasses, fmt.Errorf("invalid encrypted backup ciphertext")
	}
	key, err := derivePBKDF2SHA256([]byte(passphrase), salt, envelope.KDF.Iterations, encryptedBackupKeyLength)
	if err != nil {
		return nil, true, envelope.SecretClasses, err
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, true, envelope.SecretClasses, err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, true, envelope.SecretClasses, err
	}
	if len(nonce) != aead.NonceSize() {
		return nil, true, envelope.SecretClasses, fmt.Errorf("invalid encrypted backup nonce size")
	}
	authenticatedData := []byte(encryptedBackupFormat + ":1")
	if envelope.Version == encryptedBackupVersion {
		authenticatedData, err = encryptedBackupAuthenticatedData(envelope)
		if err != nil {
			return nil, true, envelope.SecretClasses, err
		}
	}
	plaintext, err := aead.Open(nil, nonce, ciphertext, authenticatedData)
	if err != nil {
		return nil, true, envelope.SecretClasses, fmt.Errorf("encrypted backup passphrase is incorrect or the file is damaged")
	}
	if envelope.Version == encryptedBackupLegacyV1 {
		// Version 1 did not authenticate classification metadata. Preserve payload
		// compatibility, but force callers to derive classifications from trusted
		// registered data domains instead of accepting forgeable envelope values.
		return plaintext, true, nil, nil
	}
	return plaintext, true, envelope.SecretClasses, nil
}
