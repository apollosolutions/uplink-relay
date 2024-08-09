package util

import (
	"crypto/sha256"
	"fmt"
)

func HashString(s string) string {
	return fmt.Sprintf("%x", sha256.Sum256([]byte(s)))
}
