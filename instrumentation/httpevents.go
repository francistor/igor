package instrumentation

// TODO: Integrate with metrics server

type HttpClientMetricKey struct {
	Endpoint  string
	ErrorCode string
}

type HttpClientExchangeEvent struct {
	Key HttpClientMetricKey
}

func PushHttpClientExchange(endpoint string, errorCode string) {
	MS.InputChan <- HttpClientExchangeEvent{Key: HttpClientMetricKey{Endpoint: endpoint, ErrorCode: errorCode}}
}

type HttpHandlerMetricKey struct {
	ErrorCode string
}

type HttpHandlerExchangeEvent struct {
	Key HttpHandlerMetricKey
}

func PushHttpHandlerExchange(errorCode string) {
	MS.InputChan <- HttpHandlerExchangeEvent{Key: HttpHandlerMetricKey{ErrorCode: errorCode}}
}
