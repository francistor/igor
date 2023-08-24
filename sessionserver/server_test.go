package sessionserver

import (
	"strings"
	"testing"
	"time"

	"github.com/francistor/igor/core"
	"github.com/francistor/igor/router"
)

func TestServerSunnyDay(t *testing.T) {

	// Instantiate session store
	ss := NewRadiusSessionServer("testSessionMain")

	// Radius router for sending packets to myself
	rr := router.NewRadiusRouter("testSessionMain", nil)
	rr.Start()

	accept1 := core.NewRadiusRequest(core.ACCESS_REQUEST).
		Add("Acct-Session-Id", "session1").
		Add("User-Name", "user1").
		Add("NAS-IP-Address", "1.1.1.1").
		Add("NAS-Port", 1)

	// Send radius packet
	radiusResp, err := rr.RouteRadiusRequest(accept1, "localhost:1818", time.Duration(1*time.Second), 1, 1, "secret")

	if err != nil {
		t.Fatal("got error sending access request to session server")
	}
	if radiusResp.Code != core.ACCESS_ACCEPT {
		t.Fatal("response was not access accept")
	}

	// Send query for just created session
	queryResp, err := core.HttpGet("https://localhost:18080/sessionserver/v1/sessions?index_name=User-Name&index_value=user1&active_only=false")
	if err != nil {
		t.Fatalf("query returned error %s", err)
	}
	if !strings.Contains(queryResp, "{\"SessionStore-Id\":\"session1/1.1.1.1\"}") {
		t.Fatalf("bad response to session query in accept state")
	}

	start2 := core.NewRadiusRequest(core.ACCOUNTING_REQUEST).
		Add("Acct-Session-Id", "session2").
		Add("User-Name", "user1").
		Add("NAS-IP-Address", "1.1.1.1").
		Add("NAS-Port", 2).
		Add("Framed-IP-Address", "200.0.0.2").
		Add("Acct-Status-Type", "Start")

	// Send radius packet
	radiusResp, err = rr.RouteRadiusRequest(start2, "localhost:1818", time.Duration(1*time.Second), 1, 1, "secret")

	if err != nil {
		t.Fatal("got error sending access request to session server")
	}
	if radiusResp.Code != core.ACCOUNTING_RESPONSE {
		t.Fatal("response was not accounting response")
	}

	// Send query for just created session
	queryResp, err = core.HttpGet("https://localhost:18080/sessionserver/v1/sessions?index_name=Framed-IP-Address&index_value=200.0.0.2&active_only=true")
	if err != nil {
		t.Fatalf("query returned error %s", err)
	}
	if !strings.Contains(queryResp, "{\"SessionStore-Id\":\"session2/1.1.1.1\"}") {
		t.Fatalf("bad response to session query in start state")
	}

	// Send query for both sessions
	queryResp, err = core.HttpGet("https://localhost:18080/sessionserver/v1/sessions?index_name=User-Name&index_value=user1&active_only=true")
	if err != nil {
		t.Fatalf("query returned error %s", err)
	}
	if !strings.Contains(queryResp, "{\"SessionStore-Id\":\"session2/1.1.1.1\"}") {
		t.Fatalf("bad response to session query in start state")
	}
	if !strings.Contains(queryResp, "{\"SessionStore-Id\":\"session1/1.1.1.1\"}") {
		t.Fatalf("bad response to session query in start state")
	}

	// Upgrade session to start
	start1 := core.NewRadiusRequest(core.ACCOUNTING_REQUEST).
		Add("Acct-Session-Id", "session1").
		Add("User-Name", "user1").
		Add("NAS-IP-Address", "1.1.1.1").
		Add("NAS-Port", 2).
		Add("Framed-IP-Address", "200.0.0.1").
		Add("Acct-Status-Type", "Interim-Update")

	// Send radius packet
	radiusResp, _ = rr.RouteRadiusRequest(start1, "localhost:1818", time.Duration(1*time.Second), 1, 1, "secret")
	if err != nil {
		t.Fatal("got error sending access request to session server")
	}
	if radiusResp.Code != core.ACCOUNTING_RESPONSE {
		t.Fatal("response was not accounting response")
	}

	// Send query for both sessions
	queryResp, err = core.HttpGet("https://localhost:18080/sessionserver/v1/sessions?index_name=User-Name&index_value=user1&active_only=true")
	if err != nil {
		t.Fatalf("query returned error %s", err)
	}
	if !strings.Contains(queryResp, "{\"SessionStore-Id\":\"session2/1.1.1.1\"}") {
		t.Fatalf("bad response to session query in start state")
	}
	if !strings.Contains(queryResp, "{\"Acct-Status-Type\":\"Interim-Update\"}") {
		t.Fatalf("bad response to session query in interim-update state")
	}

	ss.Close()
	rr.Close()
}
