package instrumentation

type DiameterMetrics map[DiameterMetricKey]uint64

type MetricQuery struct {
	Name   string
	Labels map[string]string // If label has a value, filter. If value is "", return in output, do not aggregate. If not present, aggregate
}

type MetricsServer struct {
	InputChan chan interface{}
	QueryChan chan interface{}

	DiameterRequestsReceived DiameterMetrics
	DiameterAnswersReceived  DiameterMetrics
	DiameterRequestsTimeout  DiameterMetrics

	DiameterRequestsSent     DiameterMetrics
	DiameterAnswersSent      DiameterMetrics
	DiameterAnswersDiscarded DiameterMetrics
}

// Returns a set of metrics in which only the properties specified in labels are not zeroed
// and the values are aggregated over the rest of labels
func GetAggDiameterMetrics(diameterMetrics DiameterMetrics, aggLabels []string) DiameterMetrics {
	outMetrics := make(DiameterMetrics)

	// Iterate through the items in the metrics map, group & add by the value of the labels
	for metricKey, v := range diameterMetrics {
		// metricKey will contain the values of the labels that we are aggregating by, the others are zeroed (not initialized)
		mk := DiameterMetricKey{}
		for _, key := range aggLabels {
			switch key {
			case "Peer":
				mk.Peer = metricKey.Peer
			case "OH":
				mk.OH = metricKey.OH
			case "OR":
				mk.OR = metricKey.OR
			case "DH":
				mk.DH = metricKey.DH
			case "DR":
				mk.DR = metricKey.DR
			case "AP":
				mk.AP = metricKey.AP
			case "CM":
				mk.CM = metricKey.CM
			}
		}
		if m, found := outMetrics[mk]; found {
			outMetrics[mk] = m + v
		} else {
			outMetrics[mk] = v
		}
	}

	return outMetrics
}

// Returns only the items in the metrics whose values correspond to the filter, which specifies
// values for certain labels
func GetFilteredDiameterMetrics(diameterMetrics DiameterMetrics, filter map[string]string) DiameterMetrics {
	outMetrics := make(DiameterMetrics)

	for metricKey := range diameterMetrics {

		// Check all the items in the filter. If mismatch, get out of the outer loop
		match := true
		for key := range filter {
			switch key {
			case "Peer":
				if metricKey.Peer != filter["Peer"] {
					match = false
					break
				}
			case "OH":
				if metricKey.OH != filter["OH"] {
					match = false
					break
				}
			case "OR":
				if metricKey.OR != filter["OR"] {
					match = false
					break
				}
			case "DH":
				if metricKey.DH != filter["DH"] {
					match = false
					break
				}
			case "DR":
				if metricKey.DR != filter["DR"] {
					match = false
					break
				}
			case "AP":
				if metricKey.AP != filter["AP"] {
					match = false
					break
				}
			case "CM":
				if metricKey.CM != filter["CM"] {
					match = false
					break
				}
			}
		}

		// Filter match
		if match {
			outMetrics[metricKey] = diameterMetrics[metricKey]
		}
	}

	return outMetrics
}

// Gets filtered and aggregated metrics
func GetDiameterMetrics(diameterMetrics DiameterMetrics, filter map[string]string, aggLabels []string) DiameterMetrics {
	var filteredMetrics DiameterMetrics
	if filter != nil {
		filteredMetrics = GetFilteredDiameterMetrics(diameterMetrics, filter)
	} else {
		// This makes a copy. Not optimal
		filteredMetrics = diameterMetrics
	}

	return GetAggDiameterMetrics(filteredMetrics, aggLabels)
}

func NewMetricsServer() *MetricsServer {
	server := MetricsServer{InputChan: make(chan interface{}), QueryChan: make(chan interface{})}

	// Initialize Metrics
	server.DiameterRequestsReceived = make(DiameterMetrics)
	server.DiameterAnswersReceived = make(DiameterMetrics)
	server.DiameterRequestsTimeout = make(DiameterMetrics)

	server.DiameterRequestsSent = make(DiameterMetrics)
	server.DiameterAnswersSent = make(DiameterMetrics)
	server.DiameterAnswersDiscarded = make(DiameterMetrics)

	// Start receive loop
	go server.storeLoop()

	return &server
}

func (ms *MetricsServer) storeLoop() {

	select {

	//case query, ok := <-ms.QueryChan:

	case event, ok := <-ms.InputChan:

		if !ok {
			break
		}

		switch e := event.(type) {

		// Diameter Events
		case DiameterRequestReceivedEvent:
			if curr, ok := ms.DiameterRequestsReceived[e.Key]; !ok {
				ms.DiameterRequestsReceived[e.Key] = 1
			} else {
				ms.DiameterRequestsReceived[e.Key] = curr + 1
			}
		case DiameterAnswerReceivedEvent:
			if curr, ok := ms.DiameterAnswersReceived[e.Key]; !ok {
				ms.DiameterAnswersReceived[e.Key] = 1
			} else {
				ms.DiameterAnswersReceived[e.Key] = curr + 1
			}
		case DiameterRequestTimeoutEvent:
			if curr, ok := ms.DiameterRequestsTimeout[e.Key]; !ok {
				ms.DiameterRequestsTimeout[e.Key] = 1
			} else {
				ms.DiameterRequestsTimeout[e.Key] = curr + 1
			}
		case DiameterRequestSentEvent:
			if curr, ok := ms.DiameterRequestsSent[e.Key]; !ok {
				ms.DiameterRequestsSent[e.Key] = 1
			} else {
				ms.DiameterRequestsSent[e.Key] = curr + 1
			}
		case DiameterAnswerSentEvent:
			if curr, ok := ms.DiameterAnswersSent[e.Key]; !ok {
				ms.DiameterAnswersSent[e.Key] = 1
			} else {
				ms.DiameterAnswersSent[e.Key] = curr + 1
			}
		case DiameterAnswerDiscardedEvent:
			if curr, ok := ms.DiameterRequestsTimeout[e.Key]; !ok {
				ms.DiameterRequestsTimeout[e.Key] = 1
			} else {
				ms.DiameterRequestsTimeout[e.Key] = curr + 1
			}

		}
	}

}
