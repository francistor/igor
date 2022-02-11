package diampeer

import (
	"fmt"
	"igor/config"
	"net"
	"time"
)

type PeerSocket struct {
	PeerConfig config.DiameterPeer

	// Internal
	connection net.Conn
}

func (p *PeerSocket) SetConnection(c net.Conn) {
	fmt.Println("passively connected")
	p.connection = c
}

func (p *PeerSocket) Connect() error {
	conn, err := net.DialTimeout("tcp4", fmt.Sprintf("%s:%d", p.PeerConfig.IPAddress, p.PeerConfig.Port), 5000*time.Millisecond)

	if err != nil {
		return err
	}
	fmt.Println("connected with " + p.PeerConfig.IPAddress)

	p.connection = conn.(*net.TCPConn)
	return nil
}

func (p *PeerSocket) ReceiveLoop() error {

	if p.connection == nil {
		return fmt.Errorf("trying to receive from a non established connection")
	}

	for {
		var receivedBytes = make([]byte, 1024)
		_, error := p.connection.Read(receivedBytes)
		if error != nil {
			return error
		} else {
			fmt.Printf("%v", receivedBytes)
		}
	}
}

func (p *PeerSocket) SendMessage(diameterMessage []byte) error {

	if p.connection == nil {
		return fmt.Errorf("trying to write to a non established connection")
	}

	_, error := p.connection.Write(diameterMessage)
	if error != nil {
		return error
	}
	return nil
}
