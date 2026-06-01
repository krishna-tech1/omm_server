package util

import (
	"crypto/rand"
)

const couponAlphabet = "ABCDEFGHJKLMNPQRSTUVWXYZ23456789"

func RandomCouponCode(length int) (string, error) {
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
		output[i] = couponAlphabet[int(buf[i])%len(couponAlphabet)]
	}

	return string(output), nil
}
