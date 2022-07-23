package radiusClient

// RadiusClientSocket
//
// Keeps track of outstanding requests in a map, which stores the destination endpoint, radiusId, response channel
// and timer, in order to match requests with answers.
//
// RadiusIds are managed per destination endpoint. The status of the 256 possible Ids for each radius destination
// is kept track in a RadiusIdManager
