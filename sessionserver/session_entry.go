package sessionserver

import (
	"github.com/francistor/igor/core"
)

const (
	PACKET_TYPE_ACCESS_REQUEST     = 1
	PACKET_TYPE_ACCOUNTING_START   = 2
	PACKET_TYPE_ACCOUNTING_INTERIM = 3
	PACKET_TYPE_ACCOUNTING_STOP    = 4
)

// Type for the sessions stored, which consist of a full radius packet received,
// and pointers to other sessions to setup a linked list ordered by expiration
// date. This is done in order to implement a session deletion stragegy that
// is as efficient as possible.
// Entries are ordered by expiration date. Newest added or updated entries
// are added to the end of the list
// An additional "expires" parameter is added to track for the expiration time
// An additional "packetType" parameter is added for easily having access to
// the session lifecycle (access - start - interim - stop)
// Sessions are added a few vendor (Session-Store) specific attributes for meta-data
// * Expires	(as Int64 milleseconds since the epoch)
// * LastUdated (as Int64 milleseconds since the epoch)
// * Id
type RadiusSessionEntry struct {
	id         string // Unique identifier of the session, composed of a combination of radius attributes
	packetType int
	packet     *core.RadiusPacket  // The radius packet
	next       *RadiusSessionEntry // Next session in the list. nil if the last
	previous   *RadiusSessionEntry // Previous session in the list. nil if the firts.
	expires    int64
}

// Linked list of Radius Sessions.
type RadiusSessionEntryList struct {
	head *RadiusSessionEntry
	tail *RadiusSessionEntry
}

// Adds a new session in the end of the Linked List
func (l *RadiusSessionEntryList) add(e *RadiusSessionEntry) {
	// Add session in the end of the list
	e.previous = l.tail
	if l.tail != nil {
		l.tail.next = e
	}
	l.tail = e

	// Update head
	if l.head == nil {
		l.head = e
	}
}

// UnLink the specified session
func (l *RadiusSessionEntryList) remove(e *RadiusSessionEntry) {

	if e.previous != nil {
		e.previous.next = e.next
	} else {
		l.head = e.next
	}

	if e.next != nil {
		e.next.previous = e.previous
	} else {
		l.tail = e.previous
	}
}
