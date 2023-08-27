package sessionserver

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/francistor/igor/core"
	"github.com/francistor/igor/router"
)

func TestSessionServerSunnyDay(t *testing.T) {

	// Instantiate session store
	ss := NewRadiusSessionServer("testSessionMain")

	// Radius router for sending packets to myself
	rr := router.NewRadiusRouter("testSessionMain", nil)
	rr.Start()

	////////////////////////////////////////////////////////////////////
	// Session 1, Accept
	////////////////////////////////////////////////////////////////////

	accept1 := core.NewRadiusRequest(core.ACCESS_REQUEST).
		Add("Acct-Session-Id", "session1").
		Add("User-Name", "user1").
		Add("NAS-IP-Address", "1.1.1.1").
		Add("NAS-Port", 1)

	// Send radius packet
	radiusResp, err := rr.RouteRadiusRequest(accept1, "localhost:1813", time.Duration(1*time.Second), 1, 1, "secret")

	if err != nil {
		t.Fatal("got error sending access request to session server")
	}
	if radiusResp.Code != core.ACCESS_ACCEPT {
		t.Fatal("response was not access accept")
	}

	// Send query for just created session
	time.Sleep(100 * time.Millisecond)
	queryResp, err := core.HttpGet("https://localhost:18813/sessionserver/v1/sessions?index_name=User-Name&index_value=user1&active_only=false")
	if err != nil {
		t.Fatalf("query returned error %s", err)
	}
	if !strings.Contains(queryResp, "{\"SessionStore-Id\":\"session1/1.1.1.1\"}") {
		t.Fatalf("bad response to session query in accept state")
	}

	////////////////////////////////////////////////////////////////////
	// Session 2, Start
	////////////////////////////////////////////////////////////////////

	start2 := core.NewRadiusRequest(core.ACCOUNTING_REQUEST).
		Add("Acct-Session-Id", "session2").
		Add("User-Name", "user1").
		Add("NAS-IP-Address", "1.1.1.1").
		Add("NAS-Port", 2).
		Add("Framed-IP-Address", "200.0.0.2").
		Add("Acct-Status-Type", "Start")

	// Send radius packet
	radiusResp, err = rr.RouteRadiusRequest(start2, "localhost:1813", time.Duration(1*time.Second), 1, 1, "secret")

	if err != nil {
		t.Fatal("got error sending access request to session server")
	}
	if radiusResp.Code != core.ACCOUNTING_RESPONSE {
		t.Fatal("response was not accounting response")
	}

	// Send query for just created session
	time.Sleep(100 * time.Millisecond)
	queryResp, err = core.HttpGet("https://localhost:18813/sessionserver/v1/sessions?index_name=Framed-IP-Address&index_value=200.0.0.2&active_only=true")
	if err != nil {
		t.Fatalf("query returned error %s", err)
	}
	if !strings.Contains(queryResp, "{\"SessionStore-Id\":\"session2/1.1.1.1\"}") {
		t.Fatalf("bad response to session query in start state")
	}

	// Send query for both sessions
	queryResp, err = core.HttpGet("https://localhost:18813/sessionserver/v1/sessions?index_name=User-Name&index_value=user1&active_only=true")
	if err != nil {
		t.Fatalf("query returned error %s", err)
	}
	if !strings.Contains(queryResp, "{\"SessionStore-Id\":\"session2/1.1.1.1\"}") {
		t.Fatalf("bad response to session query in start state")
	}
	if !strings.Contains(queryResp, "{\"SessionStore-Id\":\"session1/1.1.1.1\"}") {
		t.Fatalf("bad response to session query in start state")
	}

	////////////////////////////////////////////////////////////////////
	// Session 1, Interim
	////////////////////////////////////////////////////////////////////

	// Upgrade session to interim
	start1 := core.NewRadiusRequest(core.ACCOUNTING_REQUEST).
		Add("Acct-Session-Id", "session1").
		Add("User-Name", "user1").
		Add("NAS-IP-Address", "1.1.1.1").
		Add("NAS-Port", 2).
		Add("Framed-IP-Address", "200.0.0.1").
		Add("Acct-Status-Type", "Interim-Update")

	// Send radius packet
	radiusResp, _ = rr.RouteRadiusRequest(start1, "localhost:1813", time.Duration(1*time.Second), 1, 1, "secret")
	if err != nil {
		t.Fatal("got error sending access request to session server")
	}
	if radiusResp.Code != core.ACCOUNTING_RESPONSE {
		t.Fatal("response was not accounting response")
	}

	// Send query for both sessions
	time.Sleep(100 * time.Millisecond)
	queryResp, err = core.HttpGet("https://localhost:18813/sessionserver/v1/sessions?index_name=User-Name&index_value=user1&active_only=true")
	if err != nil {
		t.Fatalf("query returned error %s", err)
	}
	if !strings.Contains(queryResp, "{\"SessionStore-Id\":\"session2/1.1.1.1\"}") {
		t.Fatalf("bad response to session query in start state: %s", queryResp)
	}
	if !strings.Contains(queryResp, "{\"Acct-Status-Type\":\"Interim-Update\"}") {
		t.Fatalf("bad response to session query in interim-update state %s", queryResp)
	}

	////////////////////////////////////////////////////////////////////
	// Session 1, Stop
	////////////////////////////////////////////////////////////////////

	// Stop session1
	stop1 := core.NewRadiusRequest(core.ACCOUNTING_REQUEST).
		Add("Acct-Session-Id", "session1").
		Add("User-Name", "user1").
		Add("NAS-IP-Address", "1.1.1.1").
		Add("NAS-Port", 2).
		Add("Framed-IP-Address", "200.0.0.1").
		Add("Acct-Status-Type", "Stop")
	_, _ = rr.RouteRadiusRequest(stop1, "localhost:1813", time.Duration(1*time.Second), 1, 1, "secret")

	// Session is now stopped
	time.Sleep(100 * time.Millisecond)
	queryResp, err = core.HttpGet("https://localhost:18813/sessionserver/v1/sessions?index_name=User-Name&index_value=user1&active_only=true")
	if err != nil {
		t.Fatalf("query returned error %s", err)
	}
	var queryResponseObject SessionQueryResponse
	err = json.Unmarshal([]byte(queryResp), &queryResponseObject)
	if err != nil {
		t.Fatalf("could not unmarshal response to query %s", err)
	}
	if len(queryResponseObject.Items) > 1 {
		t.Fatal("got response for stopped session")
	}

	// Wait for expiration of stopped session
	// The session could be alive in the worst case 2 seconds, if the purge interval is 1 second and the limbo time is 1 second
	time.Sleep(2100 * time.Millisecond)

	// Check that the session is not available
	queryResp, err = core.HttpGet("https://localhost:18813/sessionserver/v1/sessions?index_name=User-Name&index_value=user1&active_only=false")
	if err != nil {
		t.Fatalf("query returned error %s", err)
	}

	err = json.Unmarshal([]byte(queryResp), &queryResponseObject)
	if err != nil {
		t.Fatalf("could not unmarshal response to query %s", err)
	}
	if len(queryResponseObject.Items) != 1 {
		t.Fatal("did not get exactly one session")
	}

	// Wait for expiration of started session
	// This session could be alive in the worst case for 4 seconds, if the purge interval es 1 second and the expiration time is 3 seconds
	// We need to wait 2 seconds more
	time.Sleep(2000 * time.Millisecond)
	queryResp, err = core.HttpGet("https://localhost:18813/sessionserver/v1/sessions?index_name=User-Name&index_value=user1&active_only=false")
	if err != nil {
		t.Fatalf("query returned error %s", err)
	}

	err = json.Unmarshal([]byte(queryResp), &queryResponseObject)
	if err != nil {
		t.Fatalf("could not unmarshal response to query %s", err)
	}
	if len(queryResponseObject.Items) > 0 {
		t.Fatal("at least one session was still alive")
	}

	ss.Close()
	rr.Close()
}

func TestSessionServerReplication(t *testing.T) {
	// Instantiate session stores
	ssMain := NewRadiusSessionServer("testSessionMain")
	ssReplica1 := NewRadiusSessionServer("testSessionReplica1")

	// Radius router for sending test packets
	rr := router.NewRadiusRouter("testSessionMain", nil)
	rr.Start()

	// Send accept packet to Main SessionServer
	accept1 := core.NewRadiusRequest(core.ACCESS_REQUEST).
		Add("Acct-Session-Id", "session1").
		Add("User-Name", "user1").
		Add("NAS-IP-Address", "1.1.1.1").
		Add("NAS-Port", 1)

	radiusResp, err := rr.RouteRadiusRequest(accept1, "localhost:1813", time.Duration(1*time.Second), 1, 1, "secret")
	if err != nil {
		t.Fatal("got error sending access request to session server")
	}
	if radiusResp.Code != core.ACCESS_ACCEPT {
		t.Fatal("response was not access accept")
	}

	// Session is not available in Replica1
	queryResp, err := core.HttpGet("https://localhost:18814/sessionserver/v1/sessions?index_name=User-Name&index_value=user1&active_only=false")
	if err != nil {
		t.Fatalf("query returned error %s", err)
	}
	var queryResponseObject SessionQueryResponse
	err = json.Unmarshal([]byte(queryResp), &queryResponseObject)
	if err != nil {
		t.Fatalf("could not unmarshal response to query %s", err)
	}

	// Upgrade session to start
	start1 := core.NewRadiusRequest(core.ACCOUNTING_REQUEST).
		Add("Acct-Session-Id", "session1").
		Add("User-Name", "user1").
		Add("NAS-IP-Address", "1.1.1.1").
		Add("NAS-Port", 2).
		Add("Framed-IP-Address", "200.0.0.1").
		Add("Acct-Status-Type", "Interim-Update")

	radiusResp, err = rr.RouteRadiusRequest(start1, "localhost:1813", time.Duration(1*time.Second), 1, 1, "secret")
	if err != nil {
		t.Fatal("got error sending accounting request to session server")
	}
	if radiusResp.Code != core.ACCOUNTING_RESPONSE {
		t.Fatal("response was not accounting response")
	}

	// Session is now available in Replica1
	time.Sleep(100 * time.Millisecond)
	queryResp, err = core.HttpGet("https://localhost:18814/sessionserver/v1/sessions?index_name=User-Name&index_value=user1&active_only=false")
	if err != nil {
		t.Fatalf("query returned error %s", err)
	}

	err = json.Unmarshal([]byte(queryResp), &queryResponseObject)
	if err != nil {
		t.Fatalf("could not unmarshal response to query %s", err)
	}
	fPacket := core.RadiusPacket{AVPs: queryResponseObject.Items[0]}
	if fPacket.GetStringAVP("Acct-Session-Id") != "session1" {
		t.Fatalf("bad Acct-Session-Id in replica")
	}

	// Update in replica is propagated to main
	stop1 := core.NewRadiusRequest(core.ACCOUNTING_REQUEST).
		Add("Acct-Session-Id", "session1").
		Add("User-Name", "user1").
		Add("NAS-IP-Address", "1.1.1.1").
		Add("NAS-Port", 2).
		Add("Framed-IP-Address", "200.0.0.1").
		Add("Acct-Status-Type", "Stop").
		Add("Acct-Session-Time", 3600)

	radiusResp, err = rr.RouteRadiusRequest(stop1, "localhost:1814", time.Duration(1*time.Second), 1, 1, "secret")
	if err != nil {
		t.Fatal("got error sending accounting request to replica session server")
	}
	if radiusResp.Code != core.ACCOUNTING_RESPONSE {
		t.Fatal("response was not accounting response")
	}

	// Session is now available in Master
	time.Sleep(100 * time.Millisecond)
	queryResp, err = core.HttpGet("https://localhost:18813/sessionserver/v1/sessions?index_name=User-Name&index_value=user1&active_only=false")
	if err != nil {
		t.Fatalf("query returned error %s", err)
	}

	err = json.Unmarshal([]byte(queryResp), &queryResponseObject)
	if err != nil {
		t.Fatalf("could not unmarshal response to query %s", err)
	}
	fPacket = core.RadiusPacket{AVPs: queryResponseObject.Items[0]}
	if fPacket.GetIntAVP("Acct-Session-Time") != 3600 {
		t.Fatalf("bad Acct-Session-Time in master")
	}
	if fPacket.GetStringAVP("Acct-Status-Type") != "Stop" {
		t.Fatalf("bad Acct-Session-Time in master")
	}

	ssMain.Close()
	ssReplica1.Close()
	rr.Close()
}
