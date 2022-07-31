package radiusClient

// RadiusClient
//
// Presents functions for sending requests tu upstreams servers
//
// Maintains a set of RadiusClientSockets that own the UDP socket and actually send the requests and receive the answers
//
// If the origin endpoint does not yet exist, creates a RadiusClientSocket. If the origin endpoint is not specified,
// uses one of the default RadiusClientSockets created initially
