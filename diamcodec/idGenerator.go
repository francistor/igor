package diamcodec

import (
	"fmt"
	"math/rand"
	"os"
	"strconv"
	"sync/atomic"
	"time"
)

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
	configBase := os.Getenv("IGOR_BASE")
	if configBase == "" {
		panic("environment variable IGOR_BASE undefined")
	}
	stateIdFileName := configBase + "state-id"

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
	configBase := os.Getenv("IGOR_BASE")
	if configBase == "" {
		panic("environment variable IGOR_BASE undefined")
	}
	stateIdFileName := configBase + "state-id"

	if os.WriteFile(stateIdFileName, []byte(fmt.Sprintf("%d", stateId)), 0660) != nil {
		panic("could not write state-id file")
	}

	return stateId
}
