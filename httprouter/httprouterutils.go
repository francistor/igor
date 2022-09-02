package httprouter

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"net/http"
)

// Helper function to invoke an http router operation for diameter
func RouteHTTP(client http.Client, url string, jsonRoutableRequest []byte) ([]byte, error) {

	// Send the request to the HTTP router
	httpResp, err := client.Post(url, "application/json", bytes.NewReader(jsonRoutableRequest))
	if err != nil {
		return nil, fmt.Errorf("error sending request %w", err)
	}
	defer httpResp.Body.Close()

	if httpResp.StatusCode != 200 {
		return nil, fmt.Errorf("route diameter got status code %d", httpResp.StatusCode)
	}

	jsonAnswer, err := ioutil.ReadAll(httpResp.Body)
	if err != nil {
		return nil, fmt.Errorf("error reading response %w", err)
	}

	return jsonAnswer, nil
}
