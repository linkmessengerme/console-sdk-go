package idempotency

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"io"
	"regexp"
	"strings"
)

var keyRE = regexp.MustCompile(`^[A-Za-z0-9_\-:]{1,255}$`)

func NewKey() (string, error) {
	var b [16]byte
	if _, err := io.ReadFull(rand.Reader, b[:]); err != nil {
		return "", err
	}
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80

	dst := make([]byte, 36)
	hex.Encode(dst[0:8], b[0:4])
	dst[8] = '-'
	hex.Encode(dst[9:13], b[4:6])
	dst[13] = '-'
	hex.Encode(dst[14:18], b[6:8])
	dst[18] = '-'
	hex.Encode(dst[19:23], b[8:10])
	dst[23] = '-'
	hex.Encode(dst[24:36], b[10:16])
	return string(dst), nil
}

func Validate(key string) error {
	if !keyRE.MatchString(strings.TrimSpace(key)) {
		return errors.New("idempotency key must be 1-255 characters and contain only letters, digits, underscore, dash or colon")
	}
	return nil
}
