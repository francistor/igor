package diampeer

import (
	"igor/config"
	"igor/diamcodec"
	"net"
	"os"
	"testing"
	"time"
)

func MyMessageHandler(request *diamcodec.DiameterMessage) (*diamcodec.DiameterMessage, error) {
	answer := diamcodec.NewDiameterAnswer(request)
	answer.Add("User-Name", "TestUserNameEcho")

	// Simulate the answer takes some time
	time.Sleep(300 * time.Millisecond)
	return &answer, nil
}

func TestMain(m *testing.M) {

	// Initialize logging
	config.SetupLogger()

	// Initialize the Config Object as done in main.go
	bootstrapFile := "resources/searchRules.json"
	instanceName := "testInstance"
	config.Config.Init(bootstrapFile, instanceName)

	// Execute the tests and exit
	os.Exit(m.Run())
}

func TestDiameterPeer(t *testing.T) {

	ctrlChann := make(chan struct{})

	var aPeer *DiameterPeer
	var bPeer *DiameterPeer

	/*
		aPeerConfig := config.DiameterPeer{
			DiameterHost:            "A.igor",
			IPAddress:               "127.0.0.1",
			Port:                    30000,
			ConnectionPolicy:        "passive",
			OriginNetwork:           "0.0.0.0/0",
			WatchdogIntervalMillis:  30000,
			ConnectionTimeoutMillis: 3000,
		}
	*/

	bPeerConfig := config.DiameterPeer{
		DiameterHost:            "B.igor",
		IPAddress:               "127.0.0.1",
		Port:                    30001,
		ConnectionPolicy:        "active",
		OriginNetwork:           "0.0.0.0/0",
		WatchdogIntervalMillis:  30000,
		ConnectionTimeoutMillis: 3000,
	}

	var routerInputChannel = make(chan interface{})

	// Open socket for receiving Peer connections
	listener, err := net.Listen("tcp", ":30001")
	if err != nil {
		t.Fatal(err)
	}
	go func() {
		conn, _ := listener.Accept()
		aPeer = NewPassiveDiameterPeer(routerInputChannel, conn, MyMessageHandler)
		ctrlChann <- struct{}{}
	}()

	bPeer = NewActiveDiameterPeer(routerInputChannel, bPeerConfig, MyMessageHandler)
	connectedMsg := <-routerInputChannel
	if _, ok := connectedMsg.(PeerUpEvent); !ok {
		t.Fatal("active peer did not connect")
	}
	<-ctrlChann

	// Correct response
	request, _ := diamcodec.NewDiameterRequest("TestApplication", "TestRequest")
	request.Add("User-Name", "TestUserNameRequest")
	response, error := bPeer.DiameterRequest(&request, 2*time.Second)

	if error != nil {
		t.Fatal("bad response", err)
	}
	userNameAVP, error := response.GetAVP("User-Name")
	if error != nil {
		t.Fatal("bad AVP", err)
	}
	if userNameAVP.GetString() != "TestUserNameEcho" {
		t.Fatal("bad AVP content", userNameAVP.GetString())
	}

	// Simulate a timeout. The handler takes more time than this
	_, eTimeout := bPeer.DiameterRequest(&request, 10*time.Millisecond)

	if eTimeout == nil {
		t.Fatal("should have got an error")
	} else if eTimeout.Error() != "Timeout" {
		t.Fatal("should have got a timeout")
	}

	aPeer.Close()
	bPeer.Close()
	downEvent1 := <-routerInputChannel
	if _, ok := downEvent1.(PeerDownEvent); !ok {
		t.Fatal("should have got a peerdown event")
	}
	downEvent2 := <-routerInputChannel
	if _, ok := downEvent2.(PeerDownEvent); !ok {
		t.Fatal("should have got a peerdown event")
	}
}
