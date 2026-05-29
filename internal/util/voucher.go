package util

import (
	"crypto/rand"
)

const voucherAlphabet = "ABCDEFGHJKLMNPQRSTUVWXYZ23456789"

func RandomVoucherCode(length int) (string, error) {
	if length <= 0 {
		return "", nil
	}

	output := make([]byte, length)
	buf := make([]byte, length)
	_, err := rand.Read(buf)
	if err != nil {
		return "", err
	}

	for i := 0; i < length; i++ {
		output[i] = voucherAlphabet[int(buf[i])%len(voucherAlphabet)]
	}

	return string(output), nil
}
