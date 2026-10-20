package domain

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"time"
)

// NewID returns a random UUID v4 string. IDs are always application generated;
// SQLite rowids are never exposed as business identifiers.
func NewID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic("domain: crypto/rand unavailable: " + err.Error())
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
	return string(dst)
}

// HumanNo builds a human-readable business number such as CASE-20261020-8F3A2C.
// Uniqueness is guaranteed by the database UNIQUE constraint plus 24 random bits;
// callers retry once on conflict where relevant.
func HumanNo(prefix string, now time.Time) string {
	var b [4]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic("domain: crypto/rand unavailable: " + err.Error())
	}
	return fmt.Sprintf("%s-%s-%s", prefix, now.UTC().Format("20060102"), upperHex(b[:3]))
}

func upperHex(src []byte) string {
	const digits = "0123456789ABCDEF"
	out := make([]byte, len(src)*2)
	for i, v := range src {
		out[i*2] = digits[v>>4]
		out[i*2+1] = digits[v&0x0f]
	}
	return string(out)
}

// FormatAmount renders minor units as a CNY string, e.g. 1234 -> "¥12.34".
func FormatAmount(minor int64) string {
	neg := minor < 0
	if neg {
		minor = -minor
	}
	s := fmt.Sprintf("%d.%02d", minor/100, minor%100)
	if neg {
		return "-¥" + s
	}
	return "¥" + s
}

// MaskPhone converts a raw mainland phone number into a display-safe value.
// Values that do not look like 11-digit mobiles are masked generically so no
// full number is ever persisted.
func MaskPhone(raw string) string {
	digits := make([]rune, 0, len(raw))
	for _, r := range raw {
		if r >= '0' && r <= '9' {
			digits = append(digits, r)
		}
	}
	switch {
	case len(digits) == 11:
		return string(digits[0:3]) + "****" + string(digits[7:11])
	case len(digits) > 4:
		return "****" + string(digits[len(digits)-4:])
	default:
		return "****"
	}
}

// FormatTime renders a time in UTC RFC3339 (the storage format).
func FormatTime(t time.Time) string { return t.UTC().Format(time.RFC3339) }

// ParseTime parses an RFC3339 timestamp; empty strings yield the zero time.
func ParseTime(s string) (time.Time, error) {
	if s == "" {
		return time.Time{}, nil
	}
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		return time.Time{}, err
	}
	return t.UTC(), nil
}
