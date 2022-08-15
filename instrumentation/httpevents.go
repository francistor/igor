package instrumentation

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
	Path      string
	ErrorCode string
}

type HttpHandlerExchangeEvent struct {
	Key HttpHandlerMetricKey
}

func PushHttpHandlerExchange(errorCode string, path string) {
	MS.InputChan <- HttpHandlerExchangeEvent{Key: HttpHandlerMetricKey{ErrorCode: errorCode, Path: path}}
}

type HttpRouterMetricKey struct {
	Path      string
	ErrorCode string
}

type HttpRouterExchangeEvent struct {
	Key HttpRouterMetricKey
}

func PushHttpRouterExchange(errorCode string, path string) {
	MS.InputChan <- HttpRouterExchangeEvent{Key: HttpRouterMetricKey{ErrorCode: errorCode, Path: path}}
}
