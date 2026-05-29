package util

import (
	"crypto/rand"
	"encoding/hex"
	"math/big"
)

func RandomDigits(length int) (string, error) {
	if length <= 0 {
		return "", nil
	}

	output := make([]byte, length)
	for i := 0; i < length; i++ {
		n, err := rand.Int(rand.Reader, big.NewInt(10))
		if err != nil {
			return "", err
		}
		output[i] = byte('0' + n.Int64())
	}

	return string(output), nil
}

func RandomHex(bytesLength int) (string, error) {
	if bytesLength <= 0 {
		return "", nil
	}

	buf := make([]byte, bytesLength)
	_, err := rand.Read(buf)
	if err != nil {
		return "", err
	}

	return hex.EncodeToString(buf), nil
}
