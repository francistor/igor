package sessionserver

import (
	"fmt"
	"strings"
	"time"

	"github.com/francistor/igor/core"
)

// In memory radius session store.
// Not thread safe. Must be used inside an Actor
type RadiusSessionStore struct {

	// Main map, storing by Id
	sessions map[string]*RadiusSessionEntry

	// Secondary indexes -> map of index names to-> map of index values to-> set of id(s) (implemented as map)
	indexes map[string]map[string]map[string]struct{}

	// List of sessions per type
	acceptedSessions RadiusSessionEntryList
	startedSessions  RadiusSessionEntryList
	stoppedSessions  RadiusSessionEntryList

	//////////////////////////////////////
	// Configuration

	// The expiration time for all sessions
	expirationTime time.Duration

	// Interval before deleting stopped or accepted sessions
	limboTime time.Duration

	// The names of the additional indexes, besides the id
	indexNames []string

	// The fields that compose the id
	idAttributes []string
}

// Creates a new Radius Session Store.
// For testing only. Normally will be embedded in a RadiusSessionServer.
func (s *RadiusSessionStore) init(idAttributes []string, indexNames []string, expirationTime time.Duration, limboTime time.Duration) {

	s.idAttributes = idAttributes
	s.indexNames = indexNames
	s.expirationTime = expirationTime
	s.limboTime = limboTime

	// Default index
	s.sessions = make(map[string]*RadiusSessionEntry)
	// Additional indexes
	s.indexes = make(map[string]map[string]map[string]struct{})
	for _, indexName := range s.indexNames {
		s.indexes[indexName] = make(map[string]map[string]struct{})
	}
}

// Checks if the packet passed as argument should replace the existing one for the same id
// Returns true if the new session is relevant, and optionally the session that must be
// replaced
func (s *RadiusSessionStore) CheckInsert(id string, packetType int, packet *core.RadiusPacket) (bool, *RadiusSessionEntry) {

	// Look for existing session
	existingEntry, found := s.sessions[id]

	// If no session with the same id
	if !found {
		return true, nil
	}

	// If packet type represents more recent state
	if packetType > existingEntry.packetType {
		return true, existingEntry
	}

	// If packet type represents more recent state
	if packet.GetIntAVP("Acct-Session-Time") > existingEntry.packet.GetIntAVP("Acct-Session-Time") {
		return true, existingEntry
	}

	// If date reported by the origin is newer
	if packet.GetIntAVP("Event-Timestamp") > existingEntry.packet.GetIntAVP("Event-Timestamp") {
		return true, existingEntry
	}

	return false, nil
}

// Adds a new entry to the store
// Warning: This method modifies the original packet!!!
func (s *RadiusSessionStore) PushPacket(packet *core.RadiusPacket) {

	// Build the id
	var id string
	for _, attrName := range s.idAttributes {
		id += packet.GetStringAVP(attrName)
		id += "/"
	}
	id = strings.TrimSuffix(id, "/")

	packet.Add("SessionStore-Id", id)

	// Add meta attributes
	lastUpdated := time.Now()
	packet.Add("SessionStore-LastUpdated", lastUpdated.UnixMilli())

	var packetType int
	if packet.Code == core.ACCESS_REQUEST {
		packetType = PACKET_TYPE_ACCESS_REQUEST
	} else {
		acctStatusType := packet.GetIntAVP("Acct-Status-Type")
		switch acctStatusType {
		case 1:
			// Start
			packetType = PACKET_TYPE_ACCOUNTING_START
		case 3:
			packetType = PACKET_TYPE_ACCOUNTING_INTERIM
		case 2:
			packetType = PACKET_TYPE_ACCOUNTING_STOP
		default:
			core.GetLogger().Error("received accounting packet without Acct-Status-Type")
			return
		}
	}

	// If packet is not newer, do nothing
	doInsert, oldSession := s.CheckInsert(id, packetType, packet)
	if !doInsert {
		core.GetLogger().Error("Ignoring packet")
		return
	}

	packet.Add("SessionStore-PacketType", packetType)

	var expirationTime time.Time
	if packetType == PACKET_TYPE_ACCESS_REQUEST || packetType == PACKET_TYPE_ACCOUNTING_STOP {
		expirationTime = lastUpdated.Add(s.limboTime)
	} else {
		expirationTime = lastUpdated.Add(s.expirationTime)
	}
	packet.Add("SessionStore-Expires", expirationTime.Unix())

	entry := RadiusSessionEntry{id, packetType, packet, nil, nil, expirationTime.Unix()}
	switch packetType {
	case PACKET_TYPE_ACCESS_REQUEST:
		s.acceptedSessions.add(&entry)
	case PACKET_TYPE_ACCOUNTING_START, PACKET_TYPE_ACCOUNTING_INTERIM:
		s.startedSessions.add(&entry)
	case PACKET_TYPE_ACCOUNTING_STOP:
		s.stoppedSessions.add(&entry)
	}

	if oldSession != nil {
		switch oldSession.packetType {
		case PACKET_TYPE_ACCESS_REQUEST:
			s.acceptedSessions.remove(oldSession)
		case PACKET_TYPE_ACCOUNTING_START, PACKET_TYPE_ACCOUNTING_INTERIM:
			s.startedSessions.remove(oldSession)
		}
	}

	// Add to sessions. Deleton is done always by the deleter process
	s.sessions[id] = &entry

	// Add entry for each index, if not already there. Some attributes may not be present in START
	if oldSession == nil || oldSession.packetType == PACKET_TYPE_ACCOUNTING_START {
		for _, indexName := range s.indexNames {
			// Get attribute value for the index
			indexAVP, err := packet.GetAVP(indexName)
			if err != nil {
				fmt.Println(err)
				continue
			}
			// The value is dummy. The only point of interest if the key in the map
			indexValue := indexAVP.GetString()
			if _, found := s.indexes[indexName][indexValue]; found {
				s.indexes[indexName][indexValue][id] = struct{}{}
			} else {
				s.indexes[indexName][indexValue] = make(map[string]struct{})
				s.indexes[indexName][indexValue][id] = struct{}{}
			}

		}
	}
}

// Removes the session from all the maps
func (s *RadiusSessionStore) DeleteEntry(e *RadiusSessionEntry) {

	// Unlink
	switch e.packetType {
	case PACKET_TYPE_ACCESS_REQUEST:
		s.acceptedSessions.remove(e)
	case PACKET_TYPE_ACCOUNTING_START, PACKET_TYPE_ACCOUNTING_INTERIM:
		s.startedSessions.remove(e)
	case PACKET_TYPE_ACCOUNTING_STOP:
		s.stoppedSessions.remove(e)
	}

	// Delete from indexes
	for _, indexName := range s.indexNames {
		indexAVP, _ := e.packet.GetAVP(indexName)
		indexValue := indexAVP.GetString()
		delete(s.indexes[indexName][indexValue], e.id)
		if len(s.indexes[indexName][indexValue]) == 0 {
			delete(s.indexes[indexName], indexValue)
		}
	}

	// Delete from sessions
	delete(s.sessions, e.id)
}

// Get all the sessions with the specified index name and value
func (s *RadiusSessionStore) FindByIndex(indexName string, indexValue string, activeOnly bool) []core.RadiusPacket {
	fmt.Println(s.indexes["Framed-IP-Address"])
	var ids []string
	for id := range s.indexes[indexName][indexValue] {
		ids = append(ids, id)
	}

	var packets []core.RadiusPacket
	for _, id := range ids {
		if activeOnly && s.sessions[id].packetType == PACKET_TYPE_ACCOUNTING_STOP {
			// Filter stopped sessions if so specified
			continue
		}
		packets = append(packets, *s.sessions[id].packet)
	}

	return packets
}

// Remove the expired entries from the specified list
func (s *RadiusSessionStore) ExpireEntries(list *RadiusSessionEntryList, olderThan time.Time) {

	cutoffTime := olderThan.Unix()

	entry := list.head
	for entry != nil {
		if entry.expires <= cutoffTime {
			list.remove(entry)
		}
		entry = entry.next
	}
}

func (s *RadiusSessionStore) ExpireAllEntries(olderThan time.Time) {
	s.ExpireEntries(&s.acceptedSessions, olderThan)
	s.ExpireEntries(&s.startedSessions, olderThan)
	s.ExpireEntries(&s.stoppedSessions, olderThan)
}

// Used for testing
func (s *RadiusSessionStore) GetEntries(t int, withPrint bool) []*RadiusSessionEntry {

	var entries []*RadiusSessionEntry

	var entry *RadiusSessionEntry
	switch t {
	case PACKET_TYPE_ACCESS_REQUEST:
		entry = s.acceptedSessions.head
		if withPrint {
			fmt.Println("Accepted sessions")
		}
	case PACKET_TYPE_ACCOUNTING_START, PACKET_TYPE_ACCOUNTING_INTERIM:
		entry = s.startedSessions.head
		if withPrint {
			fmt.Println("Started sessions")
		}
	case PACKET_TYPE_ACCOUNTING_STOP:
		entry = s.stoppedSessions.head
		if withPrint {
			fmt.Println("Stopped sessions")
		}
	}

	for entry != nil {
		entries = append(entries, entry)
		if withPrint {
			fmt.Println(entry)
		}
		entry = entry.next
	}

	if withPrint {
		fmt.Println("---------------")
	}

	return entries
}
