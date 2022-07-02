package instrumentation

// Buffer for the channel to receive the events
const INPUT_QUEUE_SIZE = 100

// Buffer for the channel to receive the queries
const QUERY_QUEUE_SIZE = 10

type ResetMetricsEvent struct{}

// The single instance of the metrics server
var MS *MetricsServer = NewMetricsServer()

type DiameterMetrics map[DiameterMetricKey]uint64

type Query struct {

	// Name of the metric to query
	Name string

	// List of labels to aggregate
	AggLabels []string

	// Map of label/values to filter
	Filter map[string]string

	// Channel where the response is written
	RChan chan interface{}
}

type MetricsServer struct {
	InputChan chan interface{}
	QueryChan chan Query

	// Server
	diameterRequestsReceived DiameterMetrics
	diameterAnswersSent      DiameterMetrics

	// Client
	diameterRequestsSent    DiameterMetrics
	diameterAnswersReceived DiameterMetrics
	diameterRequestsTimeout DiameterMetrics
	diameterAnswersStalled  DiameterMetrics

	// Router
	diameterRouteNotFound   DiameterMetrics
	diameterNoAvailablePeer DiameterMetrics
	diameterHandlerError    DiameterMetrics

	diameterPeersTables map[string]DiameterPeersTable
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

	// If no filter specified, do nothing
	if filter == nil {
		return diameterMetrics
	}

	// We'll put the output here
	outMetrics := make(DiameterMetrics)

	for metricKey := range diameterMetrics {

		// Check all the items in the filter. If mismatch, get out of the outer loop
		match := true
	outer:
		for key := range filter {
			switch key {
			case "Peer":
				if metricKey.Peer != filter["Peer"] {
					match = false
					break outer
				}
			case "OH":
				if metricKey.OH != filter["OH"] {
					match = false
					break outer
				}
			case "OR":
				if metricKey.OR != filter["OR"] {
					match = false
					break outer
				}
			case "DH":
				if metricKey.DH != filter["DH"] {
					match = false
					break outer
				}
			case "DR":
				if metricKey.DR != filter["DR"] {
					match = false
					break outer
				}
			case "AP":
				if metricKey.AP != filter["AP"] {
					match = false
					break outer
				}
			case "CM":
				if metricKey.CM != filter["CM"] {
					match = false
					break outer
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
	return GetAggDiameterMetrics(GetFilteredDiameterMetrics(diameterMetrics, filter), aggLabels)
}

func NewMetricsServer() *MetricsServer {
	server := MetricsServer{InputChan: make(chan interface{}, INPUT_QUEUE_SIZE), QueryChan: make(chan Query, QUERY_QUEUE_SIZE)}

	// Initialize Metrics
	server.resetMetrics()
	server.diameterPeersTables = make(map[string]DiameterPeersTable, 1)

	// Start receive loop
	go server.metricServerLoop()

	return &server
}

// Empties all the counters
func (ms *MetricsServer) resetMetrics() {
	ms.diameterRequestsReceived = make(DiameterMetrics)
	ms.diameterAnswersSent = make(DiameterMetrics)

	ms.diameterRequestsSent = make(DiameterMetrics)
	ms.diameterAnswersReceived = make(DiameterMetrics)
	ms.diameterRequestsTimeout = make(DiameterMetrics)
	ms.diameterAnswersStalled = make(DiameterMetrics)

	ms.diameterRouteNotFound = make(DiameterMetrics)
	ms.diameterNoAvailablePeer = make(DiameterMetrics)
	ms.diameterHandlerError = make(DiameterMetrics)
}

// Wrapper to reset Diameter Metrics
func (ms *MetricsServer) ResetMetrics() {
	ms.InputChan <- ResetMetricsEvent{}
}

// Wrapper to get Diameter Metrics
func (ms *MetricsServer) DiameterQuery(name string, filter map[string]string, aggLabels []string) DiameterMetrics {
	query := Query{Name: name, Filter: filter, AggLabels: aggLabels, RChan: make(chan interface{})}
	ms.QueryChan <- query
	v, ok := (<-query.RChan).(DiameterMetrics)
	if ok {
		return v
	} else {
		return DiameterMetrics{}
	}
}

// Wrapper to get PeersTable
func (ms *MetricsServer) PeersTableQuery() map[string]DiameterPeersTable {
	query := Query{Name: "DiameterPeersTables", RChan: make(chan interface{})}
	ms.QueryChan <- query
	return (<-query.RChan).(map[string]DiameterPeersTable)
}

func (ms *MetricsServer) metricServerLoop() {

	for {
		select {

		case query := <-ms.QueryChan:

			switch query.Name {
			case "DiameterRequestsReceived":
				query.RChan <- GetDiameterMetrics(ms.diameterRequestsReceived, query.Filter, query.AggLabels)
			case "DiameterAnswersSent":
				query.RChan <- GetDiameterMetrics(ms.diameterAnswersSent, query.Filter, query.AggLabels)

			case "DiameterRequestsSent":
				query.RChan <- GetDiameterMetrics(ms.diameterRequestsSent, query.Filter, query.AggLabels)
			case "DiameterAnswersReceived":
				query.RChan <- GetDiameterMetrics(ms.diameterAnswersReceived, query.Filter, query.AggLabels)
			case "DiameterRequestsTimeout":
				query.RChan <- GetDiameterMetrics(ms.diameterRequestsTimeout, query.Filter, query.AggLabels)
			case "DiameterAnswersStalled":
				query.RChan <- GetDiameterMetrics(ms.diameterAnswersSent, query.Filter, query.AggLabels)

			case "DiameterRouteNotFound":
				query.RChan <- GetDiameterMetrics(ms.diameterRouteNotFound, query.Filter, query.AggLabels)
			case "DiameterNoAvailablePeer":
				query.RChan <- GetDiameterMetrics(ms.diameterNoAvailablePeer, query.Filter, query.AggLabels)
			case "DiameterHandlerError":
				query.RChan <- GetDiameterMetrics(ms.diameterHandlerError, query.Filter, query.AggLabels)

			case "DiameterPeersTables":
				query.RChan <- ms.diameterPeersTables
			}

			close(query.RChan)

		case event, ok := <-ms.InputChan:

			if !ok {
				break
			}

			switch e := event.(type) {

			case ResetMetricsEvent:
				ms.resetMetrics()

			// Diameter Events
			case PeerDiameterRequestReceivedEvent:
				if curr, ok := ms.diameterRequestsReceived[e.Key]; !ok {
					ms.diameterRequestsReceived[e.Key] = 1
				} else {
					ms.diameterRequestsReceived[e.Key] = curr + 1
				}
			case PeerDiameterAnswerSentEvent:
				if curr, ok := ms.diameterAnswersSent[e.Key]; !ok {
					ms.diameterAnswersSent[e.Key] = 1
				} else {
					ms.diameterAnswersSent[e.Key] = curr + 1
				}

			case PeerDiameterRequestSentEvent:
				if curr, ok := ms.diameterRequestsSent[e.Key]; !ok {
					ms.diameterRequestsSent[e.Key] = 1
				} else {
					ms.diameterRequestsSent[e.Key] = curr + 1
				}

			case PeerDiameterAnswerReceivedEvent:
				if curr, ok := ms.diameterAnswersReceived[e.Key]; !ok {
					ms.diameterAnswersReceived[e.Key] = 1
				} else {
					ms.diameterAnswersReceived[e.Key] = curr + 1
				}

			case PeerDiameterRequestTimeoutEvent:
				if curr, ok := ms.diameterRequestsTimeout[e.Key]; !ok {
					ms.diameterRequestsTimeout[e.Key] = 1
				} else {
					ms.diameterRequestsTimeout[e.Key] = curr + 1
				}

			case PeerDiameterAnswerStalledEvent:
				if curr, ok := ms.diameterAnswersStalled[e.Key]; !ok {
					ms.diameterAnswersStalled[e.Key] = 1
				} else {
					ms.diameterAnswersStalled[e.Key] = curr + 1
				}

			case RouterRouteNotFoundEvent:
				if curr, ok := ms.diameterRouteNotFound[e.Key]; !ok {
					ms.diameterRouteNotFound[e.Key] = 1
				} else {
					ms.diameterRouteNotFound[e.Key] = curr + 1
				}
			case RouterNoAvailablePeerEvent:
				if curr, ok := ms.diameterNoAvailablePeer[e.Key]; !ok {
					ms.diameterNoAvailablePeer[e.Key] = 1
				} else {
					ms.diameterNoAvailablePeer[e.Key] = curr + 1
				}
			case RouterHandlerError:
				if curr, ok := ms.diameterHandlerError[e.Key]; !ok {
					ms.diameterHandlerError[e.Key] = 1
				} else {
					ms.diameterHandlerError[e.Key] = curr + 1
				}

			// PeersTable
			case DiameterPeersTableUpdatedEvent:
				ms.diameterPeersTables[e.InstanceName] = e.Table
			}
		}
	}
}
