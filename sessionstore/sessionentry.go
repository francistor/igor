package sessionstore

import (
	"github.com/francistor/igor/core"
)

const (
	PACKET_TYPE_ACCESS_REQUEST     = 1
	PACKET_TYPE_ACCOUNTING_START   = 2
	PACKET_TYPE_ACCOUNTING_INTERIM = 3
	PACKET_TYPE_ACCOUNTING_STOP    = 4
)

// Type for the sessions stored, which consist of a bunch of radius attributes
// and a pointer to the next entry.
// Entries are ordered by expiration date. Newest added or updated entries
// are added to the end of the list
type RadiusSessionEntry struct {
	id         string
	packetType int
	packet     *core.RadiusPacket
	next       *RadiusSessionEntry
	previous   *RadiusSessionEntry
	expires    int64
}

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
