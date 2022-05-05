package instrumentation

import (
	"testing"
)

func TestMetricsQuery(t *testing.T) {

	key1 := DiameterMetricKey{
		Peer: "Peer1",
		OH:   "OH1",
		OR:   "OR1",
		DH:   "DH1",
		DR:   "DR1",
		AP:   "AP1",
		CM:   "CM1",
	}

	key2 := DiameterMetricKey{
		Peer: "Peer2",
		OH:   "OH2",
		OR:   "OR2",
		DH:   "DH2",
		DR:   "DR2",
		AP:   "AP2",
		CM:   "CM2",
	}

	key3 := DiameterMetricKey{
		Peer: "Peer1",
		OH:   "OH3",
		OR:   "OR3",
		DH:   "DH3",
		DR:   "DR3",
		AP:   "AP1",
		CM:   "CM3",
	}

	key4 := DiameterMetricKey{
		Peer: "Peer1",
		OH:   "OH4",
		OR:   "OR4",
		DH:   "DH4",
		DR:   "DR4",
		AP:   "AP4",
		CM:   "CM1",
	}

	key5 := DiameterMetricKey{
		Peer: "Peer1",
		OH:   "OH1",
		OR:   "OR5",
		DH:   "DH5",
		DR:   "DR5",
		AP:   "AP5",
		CM:   "CM1",
	}

	myMetrics := DiameterMetrics{key1: 1, key2: 1, key3: 1, key4: 1, key5: 1}

	outMetrics1 := GetAggDiameterMetrics(myMetrics, []string{"Peer", "CM"})

	if outMetrics1[DiameterMetricKey{Peer: "Peer1", CM: "CM1"}] != 3 {
		t.Errorf("aggregation should be 3 but got %d", outMetrics1[DiameterMetricKey{Peer: "Peer1", CM: "CM1"}])
	}
	if outMetrics1[DiameterMetricKey{Peer: "Peer1", CM: "CM3"}] != 1 {
		t.Errorf("aggregation should be 1 but got %d", outMetrics1[DiameterMetricKey{Peer: "Peer1", CM: "CM3"}])
	}

	filteredMetrics := GetFilteredDiameterMetrics(myMetrics, map[string]string{"OH": "OH1"})
	outMetrics2 := GetAggDiameterMetrics(filteredMetrics, []string{"Peer", "CM"})
	if outMetrics2[DiameterMetricKey{Peer: "Peer1", CM: "CM1"}] != 2 {
		t.Errorf("aggregation should be 2 but got %d", outMetrics2[DiameterMetricKey{Peer: "Peer1", CM: "CM1"}])
	}
}
