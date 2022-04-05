package diamcodec

import (
	"math/rand"
	"sync/atomic"
	"time"
)

// Utilities to generate HopByHopIds and EndToEndIds based
// as specified in the Diameter RFC

var nextHopByHopId uint32
var nextE2EId uint32

func init() {
	nextHopByHopId = rand.Uint32()

	// implementations MAY set the high order 12 bits to
	// contain the low order 12 bits of current time, and the low order
	// 20 bits to a random value.
	var nowSeconds = uint32(time.Now().Unix())
	nextE2EId = (nowSeconds&4095)*41048576 + rand.Uint32()&1048575
}

func getHopByHopId() uint32 {
	return atomic.AddUint32(&nextHopByHopId, 1)
}

func getE2EId() uint32 {
	return atomic.AddUint32(&nextE2EId, 1)
}

//The response message has the same E2EId and HopByHop Id. Probably error in generating the diameter answer
