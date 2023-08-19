package sessionstore

import (
	"fmt"
	"testing"
	"time"

	"github.com/francistor/igor/core"
)

func TestSunnyDay(t *testing.T) {

	// Instantiate sesion store. Expiration times are zero
	store := NewRadiusSessionStore([]string{"Acct-Session-Id", "NAS-IP-Address"}, []string{"Framed-IP-Address"}, time.Duration(0*time.Second), time.Duration(0*time.Second))

	// Create the packets
	accessPacket1 := core.NewRadiusRequest(core.ACCESS_REQUEST).
		Add("Acct-Session-Id", "session-1").
		Add("NAS-IP-Address", "1.1.1.1")
	accessPacket2 := core.NewRadiusRequest(core.ACCESS_REQUEST).
		Add("Acct-Session-Id", "session-2").
		Add("NAS-IP-Address", "1.1.1.1")
	accessPacket3 := core.NewRadiusRequest(core.ACCESS_REQUEST).
		Add("Acct-Session-Id", "session-3").
		Add("NAS-IP-Address", "1.1.1.1")

	startPacket1 := core.NewRadiusRequest(core.ACCOUNTING_REQUEST).
		Add("Acct-Session-Id", "session-1").
		Add("NAS-IP-Address", "1.1.1.1").
		Add("Framed-IP-Address", "200.1.1.1").
		Add("Acct-Status-Type", "Start")
	startPacket2 := core.NewRadiusRequest(core.ACCOUNTING_REQUEST).
		Add("Acct-Session-Id", "session-2").
		Add("NAS-IP-Address", "1.1.1.1").
		Add("Framed-IP-Address", "200.1.1.2").
		Add("Acct-Status-Type", "Start")
	startPacket3 := core.NewRadiusRequest(core.ACCOUNTING_REQUEST).
		Add("Acct-Session-Id", "session-3").
		Add("NAS-IP-Address", "1.1.1.1").
		Add("Framed-IP-Address", "200.1.1.3").
		Add("Acct-Status-Type", "Start")

	interimPacket1 := core.NewRadiusRequest(core.ACCOUNTING_REQUEST).
		Add("Acct-Session-Id", "session-1").
		Add("NAS-IP-Address", "1.1.1.1").
		Add("Framed-IP-Address", "200.1.1.1").
		Add("Acct-Status-Type", "Interim-Update").
		Add("Acct-Session-Time", 1)
	interimPacket2 := core.NewRadiusRequest(core.ACCOUNTING_REQUEST).
		Add("Acct-Session-Id", "session-2").
		Add("NAS-IP-Address", "1.1.1.1").
		Add("Framed-IP-Address", "200.1.1.2").
		Add("Acct-Status-Type", "Interim-Update").
		Add("Acct-Session-Time", 1)
	interimPacket3 := core.NewRadiusRequest(core.ACCOUNTING_REQUEST).
		Add("Acct-Session-Id", "session-3").
		Add("NAS-IP-Address", "1.1.1.1").
		Add("Framed-IP-Address", "200.1.1.3").
		Add("Acct-Status-Type", "Interim-Update").
		Add("Acct-Session-Time", 1)

	stopPacket1 := core.NewRadiusRequest(core.ACCOUNTING_REQUEST).
		Add("Acct-Session-Id", "session-1").
		Add("NAS-IP-Address", "1.1.1.1").
		Add("Framed-IP-Address", "200.1.1.1").
		Add("Acct-Status-Type", "Stop").
		Add("Acct-Session-Time", 2)
	stopPacket2 := core.NewRadiusRequest(core.ACCOUNTING_REQUEST).
		Add("Acct-Session-Id", "session-2").
		Add("NAS-IP-Address", "1.1.1.1").
		Add("Framed-IP-Address", "200.1.1.2").
		Add("Acct-Status-Type", "Stop").
		Add("Acct-Session-Time", 2)
	stopPacket3 := core.NewRadiusRequest(core.ACCOUNTING_REQUEST).
		Add("Acct-Session-Id", "session-3").
		Add("NAS-IP-Address", "1.1.1.1").
		Add("Framed-IP-Address", "200.1.1.3").
		Add("Acct-Status-Type", "Stop").
		Add("Acct-Session-Time", 2)

	// Inject Access Requests
	fmt.Println("-----------------------------")
	fmt.Println("ACCESS")
	fmt.Println("-----------------------------")
	store.PushPacket(accessPacket1)
	store.PushPacket(accessPacket2)
	store.PushPacket(accessPacket3)
	acceptedEntries := store.GetEntries(PACKET_TYPE_ACCESS_REQUEST, true)
	if acceptedEntries[0].id != "session-1/1.1.1.1" {
		t.Fatal("incorrect first accepted session")
	}
	if acceptedEntries[2].id != "session-3/1.1.1.1" {
		t.Fatal("incorrect last accepted session")
	}

	// Inject Accounting Starts
	fmt.Println("-----------------------------")
	fmt.Println("START")
	fmt.Println("-----------------------------")
	store.PushPacket(startPacket1)
	store.PushPacket(startPacket2)
	store.PushPacket(startPacket3)
	acceptedEntries = store.GetEntries(PACKET_TYPE_ACCESS_REQUEST, true)
	if len(acceptedEntries) > 0 {
		t.Fatal("all sessions should have started")
	}
	startedEntries := store.GetEntries(PACKET_TYPE_ACCOUNTING_START, true)
	if startedEntries[0].id != "session-1/1.1.1.1" {
		t.Fatal("incorrect first accepted session")
	}
	if startedEntries[2].id != "session-3/1.1.1.1" {
		t.Fatal("incorrect last accepted session")
	}

	// Inject Accounting interims
	fmt.Println("-----------------------------")
	fmt.Println("INTERIM")
	fmt.Println("-----------------------------")
	store.PushPacket(interimPacket1)
	store.PushPacket(interimPacket2)
	store.PushPacket(interimPacket3)
	acceptedEntries = store.GetEntries(PACKET_TYPE_ACCESS_REQUEST, true)
	if len(acceptedEntries) > 0 {
		t.Fatal("all sessions should have started")
	}
	interimEntries := store.GetEntries(PACKET_TYPE_ACCOUNTING_START, true)
	if interimEntries[0].id != "session-1/1.1.1.1" {
		t.Fatal("incorrect first accepted session")
	}
	if interimEntries[2].id != "session-3/1.1.1.1" {
		t.Fatal("incorrect last accepted session")
	}
	if interimEntries[1].packet.GetIntAVP("Acct-Session-Time") != 1 {
		t.Fatalf("incorrect session time. Got %d", interimEntries[1].packet.GetIntAVP("Acct-Session-Time"))
	}

	// Find by index
	sessionsWithIPAddress := store.FindByIndex("Framed-IP-Address", "200.1.1.2", true)
	if len(sessionsWithIPAddress) != 1 {
		t.Fatal("session with IP address not found")
	}
	if sessionsWithIPAddress[0].GetStringAVP("Framed-IP-Address") != "200.1.1.2" {
		t.Fatal("incorrect IP address")
	}

	// Inject Accounting Stops
	fmt.Println("-----------------------------")
	fmt.Println("STOP")
	fmt.Println("-----------------------------")
	store.PushPacket(stopPacket1)
	store.PushPacket(stopPacket2)
	store.PushPacket(stopPacket3)
	acceptedEntries = store.GetEntries(PACKET_TYPE_ACCESS_REQUEST, true)
	if len(acceptedEntries) > 0 {
		t.Fatal("all sessions should have started and stopped")
	}
	interimEntries = store.GetEntries(PACKET_TYPE_ACCOUNTING_START, true)
	if len(interimEntries) > 0 {
		t.Fatal("all sessions should have stopped")
	}
	stoppedEntries := store.GetEntries(PACKET_TYPE_ACCOUNTING_STOP, true)
	if stoppedEntries[0].id != "session-1/1.1.1.1" {
		t.Fatal("incorrect first accepted session")
	}
	if stoppedEntries[2].id != "session-3/1.1.1.1" {
		t.Fatal("incorrect last accepted session")
	}
	if stoppedEntries[1].packet.GetIntAVP("Acct-Session-Time") != 2 {
		t.Fatalf("incorrect session time. Got %d", stoppedEntries[1].packet.GetIntAVP("Acct-Session-Time"))
	}

	// Expire
	store.ExpireAllEntries(time.Now())
	acceptedEntries = store.GetEntries(PACKET_TYPE_ACCESS_REQUEST, true)
	if len(acceptedEntries) > 0 {
		t.Fatal("all sessions should have started and stopped")
	}
	interimEntries = store.GetEntries(PACKET_TYPE_ACCOUNTING_START, true)
	if len(interimEntries) > 0 {
		t.Fatal("all sessions should have stopped")
	}
	stoppedEntries = store.GetEntries(PACKET_TYPE_ACCOUNTING_STOP, true)
	if len(stoppedEntries) > 0 {
		t.Fatal("all sessions should have been deleted")
	}
}

func TestMissingPackets(t *testing.T) {

	// Instantiate sesion store. Expiration time for limbo is zero
	store := NewRadiusSessionStore([]string{"Acct-Session-Id", "NAS-IP-Address"}, []string{"Framed-IP-Address"}, time.Duration(1*time.Second), time.Duration(0*time.Second))

	// Create the packets
	accessPacket1 := core.NewRadiusRequest(core.ACCESS_REQUEST).
		Add("Acct-Session-Id", "session-1").
		Add("NAS-IP-Address", "1.1.1.1")
	accessPacket2 := core.NewRadiusRequest(core.ACCESS_REQUEST).
		Add("Acct-Session-Id", "session-2").
		Add("NAS-IP-Address", "1.1.1.1")
	accessPacket3 := core.NewRadiusRequest(core.ACCESS_REQUEST).
		Add("Acct-Session-Id", "session-3").
		Add("NAS-IP-Address", "1.1.1.1")

	startPacket1 := core.NewRadiusRequest(core.ACCOUNTING_REQUEST).
		Add("Acct-Session-Id", "session-1").
		Add("NAS-IP-Address", "1.1.1.1").
		Add("Framed-IP-Address", "200.1.1.1").
		Add("Acct-Status-Type", "Start")
	startPacket2 := core.NewRadiusRequest(core.ACCOUNTING_REQUEST).
		Add("Acct-Session-Id", "session-2").
		Add("NAS-IP-Address", "1.1.1.1").
		Add("Framed-IP-Address", "200.1.1.2").
		Add("Acct-Status-Type", "Start")

	interimPacket1 := core.NewRadiusRequest(core.ACCOUNTING_REQUEST).
		Add("Acct-Session-Id", "session-1").
		Add("NAS-IP-Address", "1.1.1.1").
		Add("Framed-IP-Address", "200.1.1.1").
		Add("Acct-Status-Type", "Interim-Update").
		Add("Acct-Session-Time", 1)
	interimPacket2 := core.NewRadiusRequest(core.ACCOUNTING_REQUEST).
		Add("Acct-Session-Id", "session-2").
		Add("NAS-IP-Address", "1.1.1.1").
		Add("Framed-IP-Address", "200.1.1.2").
		Add("Acct-Status-Type", "Interim-Update").
		Add("Acct-Session-Time", 1)
	interimPacket3 := core.NewRadiusRequest(core.ACCOUNTING_REQUEST).
		Add("Acct-Session-Id", "session-3").
		Add("NAS-IP-Address", "1.1.1.1").
		Add("Framed-IP-Address", "200.1.1.3").
		Add("Acct-Status-Type", "Interim-Update").
		Add("Acct-Session-Time", 1)

	stopPacket1 := core.NewRadiusRequest(core.ACCOUNTING_REQUEST).
		Add("Acct-Session-Id", "session-1").
		Add("NAS-IP-Address", "1.1.1.1").
		Add("Framed-IP-Address", "200.1.1.1").
		Add("Acct-Status-Type", "Stop").
		Add("Acct-Session-Time", 2)

	// Inject Access Requests
	fmt.Println("-----------------------------")
	fmt.Println("ACCESS")
	fmt.Println("-----------------------------")
	store.PushPacket(accessPacket1)
	store.PushPacket(accessPacket2)
	store.PushPacket(accessPacket3)

	// Inject Accounting Starts. One of them is missing, so we have one accepted session and to started sessions
	fmt.Println("-----------------------------")
	fmt.Println("START")
	fmt.Println("-----------------------------")
	store.PushPacket(startPacket1)
	store.PushPacket(startPacket2)
	acceptedEntries := store.GetEntries(PACKET_TYPE_ACCESS_REQUEST, true)
	if acceptedEntries[0].id != "session-3/1.1.1.1" {
		t.Fatal("missing accepted session")
	}
	if len(acceptedEntries) != 1 {
		t.Fatal("incorrect number of accepted sessions")
	}
	startedEntries := store.GetEntries(PACKET_TYPE_ACCOUNTING_START, true)
	if startedEntries[0].id != "session-1/1.1.1.1" {
		t.Fatal("incorrect first accepted session")
	}
	if startedEntries[1].id != "session-2/1.1.1.1" {
		t.Fatal("incorrect last accepted session")
	}
	if len(startedEntries) != 2 {
		t.Fatal("incorrect number of started sessions")
	}

	// Expire the started session. The accepted session has disappeared
	store.ExpireAllEntries(time.Now())
	acceptedEntries = store.GetEntries(PACKET_TYPE_ACCESS_REQUEST, true)
	if len(acceptedEntries) != 0 {
		t.Fatal("incorrect number of accepted sessions")
	}
	startedEntries = store.GetEntries(PACKET_TYPE_ACCOUNTING_START, true)
	if startedEntries[0].id != "session-1/1.1.1.1" {
		t.Fatal("incorrect first accepted session")
	}
	if startedEntries[1].id != "session-2/1.1.1.1" {
		t.Fatal("incorrect last accepted session")
	}
	if len(startedEntries) != 2 {
		t.Fatal("incorrect number of started sessions")
	}

	// Inject Accounting interims. Now we get the missing interim
	fmt.Println("-----------------------------")
	fmt.Println("INTERIM")
	fmt.Println("-----------------------------")
	store.PushPacket(interimPacket1)
	store.PushPacket(interimPacket2)
	store.PushPacket(interimPacket3)
	acceptedEntries = store.GetEntries(PACKET_TYPE_ACCESS_REQUEST, true)
	if len(acceptedEntries) > 0 {
		t.Fatal("all sessions should have started")
	}
	interimEntries := store.GetEntries(PACKET_TYPE_ACCOUNTING_START, true)
	if interimEntries[0].id != "session-1/1.1.1.1" {
		t.Fatal("incorrect first accepted session")
	}
	if interimEntries[2].id != "session-3/1.1.1.1" {
		t.Fatal("incorrect last accepted session")
	}
	if interimEntries[1].packet.GetIntAVP("Acct-Session-Time") != 1 {
		t.Fatalf("incorrect session time. Got %d", interimEntries[1].packet.GetIntAVP("Acct-Session-Time"))
	}

	// Inject Accounting Stops
	fmt.Println("-----------------------------")
	fmt.Println("STOP")
	fmt.Println("-----------------------------")
	store.PushPacket(stopPacket1)
	acceptedEntries = store.GetEntries(PACKET_TYPE_ACCESS_REQUEST, true)
	if len(acceptedEntries) > 0 {
		t.Fatal("all sessions should have started and stopped")
	}
	interimEntries = store.GetEntries(PACKET_TYPE_ACCOUNTING_START, true)
	if len(interimEntries) != 2 {
		t.Fatal("there must be 2 started sessions")
	}
	stoppedEntries := store.GetEntries(PACKET_TYPE_ACCOUNTING_STOP, true)
	if stoppedEntries[0].id != "session-1/1.1.1.1" {
		t.Fatal("incorrect first accepted session")
	}
	if len(stoppedEntries) != 1 {
		t.Fatal("there must be 1 stopped session")
	}

	// Expire
	store.ExpireAllEntries(time.Now())
	acceptedEntries = store.GetEntries(PACKET_TYPE_ACCESS_REQUEST, true)
	if len(acceptedEntries) > 0 {
		t.Fatal("all sessions should have started and stopped")
	}
	interimEntries = store.GetEntries(PACKET_TYPE_ACCOUNTING_START, true)
	if len(interimEntries) != 2 {
		t.Fatal("there must be 2 started sessions")
	}
	stoppedEntries = store.GetEntries(PACKET_TYPE_ACCOUNTING_STOP, true)
	if len(stoppedEntries) > 0 {
		t.Fatal("all stopped sessions should have been deleted")
	}

	// Force expiration of all
	store.ExpireAllEntries(time.Now().Add(time.Duration(1 * time.Second)))
	acceptedEntries = store.GetEntries(PACKET_TYPE_ACCESS_REQUEST, true)
	if len(acceptedEntries) > 0 {
		t.Fatal("all sessions should have started and stopped")
	}
	interimEntries = store.GetEntries(PACKET_TYPE_ACCOUNTING_START, true)
	if len(interimEntries) > 0 {
		t.Fatal("all sessions should have been deleted")
	}
	stoppedEntries = store.GetEntries(PACKET_TYPE_ACCOUNTING_STOP, true)
	if len(stoppedEntries) > 0 {
		t.Fatal("all stopped sessions should have been deleted")
	}
}

func TestMultipleIndexValues(t *testing.T) {

	// Instantiate sesion store.
	store := NewRadiusSessionStore([]string{"Acct-Session-Id", "NAS-IP-Address"}, []string{"Framed-IP-Address"}, time.Duration(1*time.Second), time.Duration(0*time.Second))

	startPacket1 := core.NewRadiusRequest(core.ACCOUNTING_REQUEST).
		Add("Acct-Session-Id", "session-1").
		Add("NAS-IP-Address", "1.1.1.1").
		Add("Framed-IP-Address", "9.9.9.9").
		Add("Acct-Status-Type", "Start")
	startPacket2 := core.NewRadiusRequest(core.ACCOUNTING_REQUEST).
		Add("Acct-Session-Id", "session-2").
		Add("NAS-IP-Address", "1.1.1.1").
		Add("Framed-IP-Address", "9.9.9.9").
		Add("Acct-Status-Type", "Start")

	store.PushPacket(startPacket1)
	store.PushPacket(startPacket2)

	sessions := store.FindByIndex("Framed-IP-Address", "9.9.9.9", true)
	if len(sessions) != 2 {
		t.Fatal("number of sessions with the same IP adress was not 2")
	}
	if sessions[1].GetStringAVP("Framed-IP-Address") != "9.9.9.9" {
		t.Fatal("IP address found is not 9.9.9.9")
	}

	// Stop one session
	stopPacket1 := core.NewRadiusRequest(core.ACCOUNTING_REQUEST).
		Add("Acct-Session-Id", "session-1").
		Add("NAS-IP-Address", "1.1.1.1").
		Add("Framed-IP-Address", "9.9.9.9").
		Add("Acct-Status-Type", "Stop")

	store.PushPacket(stopPacket1)

	// With stop filtered
	sessions = store.FindByIndex("Framed-IP-Address", "9.9.9.9", true)
	if len(sessions) != 1 {
		t.Fatal("number of sessions with the same IP adress was not 1")
	}

	// Without stop filtered
	sessions = store.FindByIndex("Framed-IP-Address", "9.9.9.9", false)
	if len(sessions) != 2 {
		t.Fatal("number of sessions with the same IP adress was not 2")
	}

}
