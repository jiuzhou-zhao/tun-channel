package pkg

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/sha256"
	"errors"
)

type EnDecrypt interface {
	Encrypt([]byte) []byte
	Decrypt([]byte) ([]byte, error)
}

type NonEnDecrypt struct{}

func (edc *NonEnDecrypt) Encrypt(d []byte) []byte {
	return d
}

func (edc *NonEnDecrypt) Decrypt(d []byte) ([]byte, error) {
	return d, nil
}

type AESEnDecrypt struct {
	key []byte
}

func NewAESEnDecrypt(key string) *AESEnDecrypt {
	rKey := sha256.Sum256([]byte(key))
	return &AESEnDecrypt{
		key: rKey[:16],
	}
}

func (edc *AESEnDecrypt) pkcs7Padding(text []byte, blockSize int) []byte {
	padding := blockSize - len(text)%blockSize
	return append(text, bytes.Repeat([]byte{byte(padding)}, padding)...)
}

func (edc *AESEnDecrypt) pkcs7UnPadding(origData []byte) ([]byte, error) {
	length := len(origData)
	if length == 0 {
		return nil, errors.New("bad data")
	}
	return origData[:(length - int(origData[length-1]))], nil
}

func (edc *AESEnDecrypt) Encrypt(d []byte) []byte {
	block, _ := aes.NewCipher(edc.key)
	blockSize := block.BlockSize()
	d = edc.pkcs7Padding(d, blockSize)
	blocMode := cipher.NewCBCEncrypter(block, edc.key[:blockSize])
	cd := make([]byte, len(d))
	blocMode.CryptBlocks(cd, d)
	return cd
}

func (edc *AESEnDecrypt) Decrypt(d []byte) ([]byte, error) {
	block, err := aes.NewCipher(edc.key)
	if err != nil {
		return nil, err
	}
	blockSize := block.BlockSize()
	blockMode := cipher.NewCBCDecrypter(block, edc.key[:blockSize])
	origData := make([]byte, len(d))
	blockMode.CryptBlocks(origData, d)
	origData, err = edc.pkcs7UnPadding(origData)
	if err != nil {
		return nil, err
	}
	return origData, err
}
