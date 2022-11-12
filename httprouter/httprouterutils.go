package httprouter

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
)

// Helper function to invoke an http router operation for radius and diameter
func RouteHttp(client http.Client, url string, jsonRoutableRequest []byte) ([]byte, error) {

	// Send the request to the HTTP router
	httpResp, err := client.Post(url, "application/json", bytes.NewReader(jsonRoutableRequest))
	if err != nil {
		return nil, fmt.Errorf("error sending request %w", err)
	}
	defer httpResp.Body.Close()

	if httpResp.StatusCode != 200 {
		return nil, fmt.Errorf("route request got status code %d", httpResp.StatusCode)
	}

	jsonAnswer, err := io.ReadAll(httpResp.Body)
	if err != nil {
		return nil, fmt.Errorf("error reading response %w", err)
	}

	return jsonAnswer, nil
}
