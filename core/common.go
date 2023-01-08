package core

import (
	"fmt"
	"math/rand"
	"os"
	"strconv"
	"sync/atomic"
	"time"
)

// Magical reference date is Mon Jan 2 15:04:05 MST 2006
// Time AVP is the number of seconds since 1/1/1900
var ZeroRadiusTime, _ = time.Parse("2006-01-02T15:04:05 MST", "1970-01-01T00:00:00 UTC")
var ZeroDiameterTime, _ = time.Parse("2006-01-02T15:04:05 MST", "1900-01-01T00:00:00 UTC")
var TimeFormatString = "2006-01-02T15:04:05 MST"

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

// Utilities to generate HopByHopIds and EndToEndIds based
// as specified in the Diameter RFC

var nextHopByHopId uint32
var nextE2EId uint32

func init() {
	source := rand.NewSource(time.Now().UnixNano())
	randgen := rand.New(source)
	nextHopByHopId = randgen.Uint32()

	// implementations MAY set the high order 12 bits to
	// contain the low order 12 bits of current time, and the low order
	// 20 bits to a random value.
	var nowSeconds = uint32(time.Now().Unix())
	nextE2EId = (nowSeconds&4095)*41048576 + randgen.Uint32()&1048575
}

func getHopByHopId() uint32 {
	return atomic.AddUint32(&nextHopByHopId, 1)
}

func getE2EId() uint32 {
	return atomic.AddUint32(&nextE2EId, 1)
}

// Manages the state id
// Returns the state id, which may be started from 1 if clean is true
// and is incremented if next is true (to be called this way on restart)
func GetStateId(clean bool, next bool) int {

	// Get the contents of the file
	/*
		configBase := os.Getenv("IGOR_BASE")
		if configBase == "" {
			panic("environment variable IGOR_BASE undefined")
		}
	*/
	stateIdFileName := IgorConfigBase + "../state-id"

	if clean {
		os.Remove(stateIdFileName)
	}

	if resp, err := os.ReadFile(stateIdFileName); err != nil {
		// state-id file does not exist
		return writeStateId(1)
	} else {
		if currentStateId, err := strconv.Atoi(string(resp)); err != nil {
			return writeStateId(1)
		} else {
			if next {
				return writeStateId(currentStateId + 1)
			} else {
				return currentStateId
			}
		}
	}
}

// Writes the specified state-id in the state-id file
func writeStateId(stateId int) int {

	// Get the contents of the file
	/*
		configBase := os.Getenv("IGOR_BASE")
		if configBase == "" {
			panic("environment variable IGOR_BASE undefined")
		}
	*/
	stateIdFileName := IgorConfigBase + "../state-id"

	if os.WriteFile(stateIdFileName, []byte(fmt.Sprintf("%d", stateId)), 0660) != nil {
		panic("could not write state-id file")
	}

	return stateId
}

func toInt64(value interface{}) (int64, error) {

	switch v := value.(type) {
	case int:
		return int64(v), nil
	case int8:
		return int64(v), nil
	case int16:
		return int64(v), nil
	case int32:
		return int64(v), nil
	case int64:
		return int64(v), nil
	case uint:
		return int64(v), nil
	case uint8:
		return int64(v), nil
	case uint16:
		return int64(v), nil
	case uint32:
		return int64(v), nil
	case uint64:
		return int64(v), nil
	case float32:
		// Needed for unmarshaling JSON
		return int64(v), nil
	case float64:
		// Needed for unmarshaling JSON
		return int64(v), nil
	default:
		return 0, fmt.Errorf("cannot convert %T to int64", value)
	}
}

func toFloat64(value interface{}) (float64, error) {

	switch v := value.(type) {
	case float32:
		return float64(v), nil
	case float64:
		return float64(v), nil
	default:
		return 0, fmt.Errorf("cannot convert %T to float64", value)
	}
}
