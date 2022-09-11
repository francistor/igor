# igor
Go AAA Server

## Diameter message lifecycle

Case A. Diameter message receive from another Diameter entity

* The message is received from another Peer in the `diampeer` type
* If it is a request and not a Base application message, the handler is invoked.
  * The handler must have a context parameter
* The handler for a Diameter Peer invokes `router.RouteDiameterRequest(request, DEFAULT_REQUEST_TIMEOUT_SECONDS*time.Second)`
  * `DiameterRouter.RouteDiameterRequest` must have a conext parameter

