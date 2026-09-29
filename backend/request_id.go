package main

import (
	"crypto/rand"
	"encoding/hex"
)

func newRequestID() string {
	var value [8]byte
	if _, err := rand.Read(value[:]); err != nil {
		return "local-request"
	}
	return hex.EncodeToString(value[:])
}
