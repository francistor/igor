# Igor

Igor is an AAA server and client built using Go. It implements Radius and Diameter protocols.

This repo contains Igor core, a library on top of which fully fledged servers (Igor instances) can be built. See `igor-psba` and `igor-client`.

Igor offers two alternatives for building the algorigthms that treat the Radius or Diameter requests:

* Handle them in Go functions, implementing `type RadiusPacketHandler func(request *RadiusPacket) (*RadiusPacket, error)` or `type MessageHandler func(request *DiameterMessage) (*DiameterMessage, error)`

* Handle them as http2 requests, with a JSON payload representing the radius packet or diameter message, producing an answer which will respresent the answer. Any web server can be used for this purpose, and standard http message processing techniques may be applied. As a drawback, the performance is much lower.

In addition, Igor accepts also http requests that represent Radius or Diameter messages, which will be processed according to the defined rules, for instance, forwarding to another Diameter peer or Radius server. In this way, it will acct as a Radius client.

The Igor library contains:

* Configuration utilities that read from files, http or database tables the core parameters of the system, such as ports, peers, radius secrets, etc.
* Additional configuration utilities that help in the treatment of standard Radius user files
* More configuration utilities that create template based configurations, with parameters taken from files or databases
* Implementation of a Diameter Peer, which establishes a relationship with other Peers (exchanging capabilities and watchdog messages) and provides hooks for the received messages and functions to send messages treating the timeout
* Implementation of a Radius Server, which receives the requests and invokes a hook for processing
* Impelentation of a Radius Client, that sends radius messages to an uptream server and receives the answer, treating the timeout
* Radius and Diameter routers, which receive the messages and decide which action to take: send to a peer, send to a local (go) or remote (http2) handler, and manage the status of the upstream servers. The http router module implements an http2 interface for the router to receive radius packets and Diameter messages
* A sample Http2 handler, which un-serializes back the json/http2 message to Diameter or Radius and invoke a go handler, but in another process from the one that received the message
* Instrumentation utilities for cooking metrics and exporting them in Prometheus format
* Utilities for filtering and manipulating radius packets
* Utilities for writing radius packets and diameter messages to file

## Configuration

Configuration is done using files, which may reside locally or in an http location, and optionally may be created dynamicaly with a database query.
