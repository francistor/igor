package radiuscodec

import (
	"math/rand"
	"time"
)

func GetAuthenticator() [16]byte {
	var authenticator [16]byte
	rand.Seed(time.Now().UnixNano())
	rand.Read(authenticator[:])
	return authenticator
}

func GetSalt() [2]byte {
	salt := make([]byte, 2)
	rand.Seed(time.Now().UnixNano())
	rand.Read(salt)
	return [2]byte{salt[0], salt[1]}
}
