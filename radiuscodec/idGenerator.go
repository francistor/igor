package radiuscodec

import (
	"math/rand"
	"time"
)

// Utilities to generate random Authenticator

func GetAuthenticator() []byte {
	authenticator := make([]byte, 16)

	rand.Seed(time.Now().UnixNano())

	rand.Read(authenticator)

	return authenticator
}
