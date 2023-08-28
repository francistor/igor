package core

import (
	"testing"
)

func TestDiameterAggregations(t *testing.T) {

	key1 := PeerDiameterMetricKey{
		Peer: "Peer1",
		OH:   "OH1",
		OR:   "OR1",
		DH:   "DH1",
		DR:   "DR1",
		AP:   "AP1",
		CM:   "CM1",
	}

	key2 := PeerDiameterMetricKey{
		Peer: "Peer2",
		OH:   "OH2",
		OR:   "OR2",
		DH:   "DH2",
		DR:   "DR2",
		AP:   "AP2",
		CM:   "CM2",
	}

	key3 := PeerDiameterMetricKey{
		Peer: "Peer1",
		OH:   "OH3",
		OR:   "OR3",
		DH:   "DH3",
		DR:   "DR3",
		AP:   "AP1",
		CM:   "CM3",
	}

	key4 := PeerDiameterMetricKey{
		Peer: "Peer1",
		OH:   "OH4",
		OR:   "OR4",
		DH:   "DH4",
		DR:   "DR4",
		AP:   "AP4",
		CM:   "CM1",
	}

	key5 := PeerDiameterMetricKey{
		Peer: "Peer1",
		OH:   "OH1",
		OR:   "OR5",
		DH:   "DH5",
		DR:   "DR5",
		AP:   "AP5",
		CM:   "CM1",
	}

	myMetrics := PeerDiameterMetrics{key1: 1, key2: 1, key3: 1, key4: 1, key5: 1}

	outMetrics1 := GetAggPeerDiameterMetrics(myMetrics, []string{"Peer", "CM"})

	if outMetrics1[PeerDiameterMetricKey{Peer: "Peer1", CM: "CM1"}] != 3 {
		t.Errorf("aggregation should be 3 but got %d", outMetrics1[PeerDiameterMetricKey{Peer: "Peer1", CM: "CM1"}])
	}
	if outMetrics1[PeerDiameterMetricKey{Peer: "Peer1", CM: "CM3"}] != 1 {
		t.Errorf("aggregation should be 1 but got %d", outMetrics1[PeerDiameterMetricKey{Peer: "Peer1", CM: "CM3"}])
	}

	filteredMetrics := GetFilteredPeerDiameterMetrics(myMetrics, map[string]string{"OH": "OH1"})
	outMetrics2 := GetAggPeerDiameterMetrics(filteredMetrics, []string{"Peer", "CM"})
	if outMetrics2[PeerDiameterMetricKey{Peer: "Peer1", CM: "CM1"}] != 2 {
		t.Errorf("aggregation should be 2 but got %d", outMetrics2[PeerDiameterMetricKey{Peer: "Peer1", CM: "CM1"}])
	}
}

func TestHttpClientAggregations(t *testing.T) {

	key1 := HttpClientMetricKey{
		Endpoint:  "http://localhost1",
		ErrorCode: "200",
	}

	key2 := HttpClientMetricKey{
		Endpoint:  "http://localhost2",
		ErrorCode: "200",
	}

	key3 := HttpClientMetricKey{
		Endpoint:  "http://localhost2",
		ErrorCode: "500",
	}

	myMetrics := HttpClientMetrics{key1: 1, key2: 1, key3: 1}

	outMetrics := GetHttpClientMetrics(myMetrics, map[string]string{"ErrorCode": "200"}, []string{"ErrorCode"})
	if outMetrics[HttpClientMetricKey{ErrorCode: "200"}] != 2 {
		t.Errorf("aggregation should be 2 but got %d", outMetrics[HttpClientMetricKey{ErrorCode: "200"}])
	}
}

func TestHttpHandlerAggregations(t *testing.T) {

	key1 := HttpHandlerMetricKey{
		ErrorCode: "200",
	}

	key2 := HttpHandlerMetricKey{
		ErrorCode: "500",
	}

	myMetrics := HttpHandlerMetrics{key1: 1, key2: 1}

	outMetrics := GetHttpHandlerMetrics(myMetrics, map[string]string{"ErrorCode": "200"}, []string{"ErrorCode"})
	if outMetrics[HttpHandlerMetricKey{ErrorCode: "200"}] != 1 {
		t.Errorf("aggregation should be 2 but got %d", outMetrics[HttpHandlerMetricKey{ErrorCode: "200"}])
	}
}

func TestRadiusAggregations(t *testing.T) {

	key1 := RadiusMetricKey{
		Endpoint: "127.0.0.1:1812",
		Code:     "1",
	}

	key2 := RadiusMetricKey{
		Endpoint: "127.0.0.1:1812",
		Code:     "2",
	}

	myMetrics := RadiusMetrics{key1: 1, key2: 1}

	outMetrics := GetRadiusMetrics(myMetrics, map[string]string{"Endpoint": "127.0.0.1:1812"}, []string{"Code"})
	if outMetrics[RadiusMetricKey{Code: "1"}] != 1 {
		t.Errorf("aggregation should be 1 but got %d", outMetrics[RadiusMetricKey{Code: "1"}])
	}

	outMetrics = GetRadiusMetrics(myMetrics, map[string]string{"Endpoint": "127.0.0.1:1812"}, []string{})
	if outMetrics[RadiusMetricKey{}] != 2 {
		t.Errorf("aggregation should be 2 but got %d", outMetrics[RadiusMetricKey{}])
	}
}
