package sessionserver

import (
	"fmt"
	"strings"
	"time"

	"github.com/francistor/igor/core"
)

// In memory radius session store.
// Not thread safe. Must be used inside an Actor.
// The desing and usage principles are
// * The id must be globally unique and unique over time
// * Multiple RadiusSessionStore instances will be normally running in parallel, each one
// possibly having incomplete information. It is up to the clients to query multiple
// RadiusSesionStore instances and merge the entries received in order to get the most
// recent information.
// For instance, to get the session with an existing IP address, still the id should be
// a radius session-id, which is unique. IP addresses are not. If the query returns a
// session which has been updated later, but whose implied start session is more recent,
// it should take this one.
//
// It may also be used to enforce uniquenes of certain session attributes. The method
// for inserting a session will inform of that condition if a unique index is specified
// and a duplicate is found.
type RadiusSessionStore struct {

	// List of attribute names to store
	attributes []string

	// Main map, storing by Id
	sessions map[string]*RadiusSessionEntry

	// Secondary indexes -> map of index names to-> map of index values to-> set of id(s) (implemented as map)
	indexes map[string]map[string]map[string]struct{}

	// List of sessions per type and ordered by expiration time
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
	indexConf []core.SessionIndexConf

	// The fields that compose the id
	idAttributes []string
}

// Creates a new Radius Session Store.
// For testing only. Normally will be embedded in a RadiusSessionServer.
func (s *RadiusSessionStore) init(attributes []string, idAttributes []string, indexConf []core.SessionIndexConf, expirationTime time.Duration, limboTime time.Duration) {

	s.attributes = attributes
	s.idAttributes = idAttributes
	s.indexConf = indexConf
	s.expirationTime = expirationTime
	s.limboTime = limboTime

	// Default index
	s.sessions = make(map[string]*RadiusSessionEntry)
	// Additional indexes
	s.indexes = make(map[string]map[string]map[string]struct{})
	for _, indexConf := range s.indexConf {
		s.indexes[indexConf.IndexName] = make(map[string]map[string]struct{})
	}
}

// Checks if the packet passed as argument should replace the existing one for the same id.
// Returns true if the new session is relevant, and optionally the session that must be
// replaced.
func (s *RadiusSessionStore) checkInsert(id string, packetType int, packet *core.RadiusPacket) (bool, *RadiusSessionEntry) {

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

// Adds a new entry to the store.
// Returns the offending session-id and index if a restriction was found.
func (s *RadiusSessionStore) PushPacket(packet *core.RadiusPacket) (string, string) {

	// Build the id
	var id string
	for _, attrName := range s.idAttributes {
		id += packet.GetStringAVP(attrName)
		id += "/"
	}
	id = strings.TrimSuffix(id, "/")

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
			return "", ""
		}
	}

	// If packet is not newer, do nothing
	doInsert, oldSession := s.checkInsert(id, packetType, packet)
	if !doInsert {
		core.GetLogger().Warn("Ignoring packet. New: %s Old: %s", packet, oldSession)
		return "", ""
	}

	// Check unique indexes
	if packetType == PACKET_TYPE_ACCESS_REQUEST {
		for _, indexConf := range s.indexConf {
			if indexConf.IsUnique {
				indexName := indexConf.IndexName
				// Get attribute value for the index
				indexAVP, err := packet.GetAVP(indexName)
				if err != nil {
					continue
				}
				indexValue := indexAVP.GetString()
				if sessionIds, found := s.indexes[indexName][indexValue]; found {
					// Should only be one entry, but iterate through all
					for id := range sessionIds {
						if entry, found := s.sessions[id]; found {
							// If type indicates that the session is not terminated, it is ok to go on
							if entry.packetType != PACKET_TYPE_ACCOUNTING_STOP {
								return id, indexName
							}
						}
					}
				}
			}
		}
	}

	// Copy of the packet that will be stored
	packetCopy := packet.Copy(s.attributes, nil)

	// Add meta attributes
	lastUpdated := time.Now()
	var expirationTime time.Time
	if packetType == PACKET_TYPE_ACCESS_REQUEST || packetType == PACKET_TYPE_ACCOUNTING_STOP {
		expirationTime = lastUpdated.Add(s.limboTime)
	} else {
		expirationTime = lastUpdated.Add(s.expirationTime)
	}
	packetCopy.Add("SessionStore-Expires", expirationTime.UnixMilli())
	packetCopy.Add("SessionStore-LastUpdated", lastUpdated.UnixMilli())
	packetCopy.Add("SessionStore-Id", id)

	// Object to be stored
	entry := RadiusSessionEntry{id, packetType, packetCopy, nil, nil, expirationTime.UnixMilli()}

	// There is a linked list for each type of session
	// Add the new session
	switch packetType {
	case PACKET_TYPE_ACCESS_REQUEST:
		s.acceptedSessions.add(&entry)

	case PACKET_TYPE_ACCOUNTING_START, PACKET_TYPE_ACCOUNTING_INTERIM:
		s.startedSessions.add(&entry)

	case PACKET_TYPE_ACCOUNTING_STOP:
		s.stoppedSessions.add(&entry)
	}

	// Remove the old session
	if oldSession != nil {
		switch oldSession.packetType {

		case PACKET_TYPE_ACCESS_REQUEST:
			s.acceptedSessions.remove(oldSession)

		case PACKET_TYPE_ACCOUNTING_START, PACKET_TYPE_ACCOUNTING_INTERIM:
			s.startedSessions.remove(oldSession)
		}
	}

	// Add to sessions. Deletion is done always by the deleter process
	s.sessions[id] = &entry

	// Add entry for each index, if not already there. Some attributes may not be present in an access request
	if oldSession == nil || oldSession.packetType == PACKET_TYPE_ACCESS_REQUEST {
		for _, indexConf := range s.indexConf {
			indexName := indexConf.IndexName
			// Get attribute value for the index
			indexAVP, err := packet.GetAVP(indexName)
			if err != nil {
				// Attribute for index not found
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

	return "", ""
}

// Removes the session from all the maps
func (s *RadiusSessionStore) deleteEntry(e *RadiusSessionEntry) {

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
	for _, indexConf := range s.indexConf {
		indexName := indexConf.IndexName
		if indexAVP, err := e.packet.GetAVP(indexName); err == nil {
			indexValue := indexAVP.GetString()
			delete(s.indexes[indexName][indexValue], e.id)
			if len(s.indexes[indexName][indexValue]) == 0 {
				delete(s.indexes[indexName], indexValue)
			}
		}
	}

	// Delete from sessions
	delete(s.sessions, e.id)
}

// Remove the expired entries from the specified list
func (s *RadiusSessionStore) expireEntries(list *RadiusSessionEntryList, olderThan time.Time) {

	cutoffTime := olderThan.UnixMilli()

	entry := list.head
	for entry != nil {
		if entry.expires <= cutoffTime {
			s.deleteEntry(entry)
		}
		entry = entry.next
	}
}

// The method arguments should normally be set to now(), except for testing
func (s *RadiusSessionStore) expireAllEntries(currentExpireDate time.Time, currentLimboDate time.Time) {
	s.expireEntries(&s.acceptedSessions, currentLimboDate)
	s.expireEntries(&s.startedSessions, currentExpireDate)
	s.expireEntries(&s.stoppedSessions, currentLimboDate)
}

// Returns the total number of sessions
func (s *RadiusSessionStore) getCount() int {
	return len(s.sessions)
}

// Get all the sessions with the specified index name and value
func (s *RadiusSessionStore) FindByIndex(indexName string, indexValue string, activeOnly bool) []core.RadiusPacket {

	index, found := s.indexes[indexName]
	if !found {
		core.GetLogger().Warnf("query for non existing index: %s", indexName)
		return nil
	}

	var ids []string
	for id := range index[indexValue] {
		ids = append(ids, id)
	}

	var packets = make([]core.RadiusPacket, 0)
	for _, id := range ids {
		if activeOnly && s.sessions[id].packetType == PACKET_TYPE_ACCOUNTING_STOP {
			// Filter stopped sessions if so specified
			continue
		}
		packets = append(packets, *s.sessions[id].packet)
	}

	return packets
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
