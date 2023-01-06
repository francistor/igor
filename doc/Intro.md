# Igor

Igor is an AAA server and client built using Go. It implements Radius and Diameter protocols.

This repo contains Igor core, a library on top of which fully fledged servers (Igor instances) can be built. See `igor-psba` and `igor-client`.

Igor offers two alternatives for building the algorigthms that treat the Radius or Diameter requests:

* Handle them in Go functions, implementing `type RadiusPacketHandler func(request *RadiusPacket) (*RadiusPacket, error)` or `type MessageHandler func(request *DiameterMessage) (*DiameterMessage, error)`

* Handle them as http2 requests, with a JSON payload representing the radius packet or diameter message, producing an answer which will respresent the answer. Any web server can be used for this purpose, and standard http message processing techniques may be applied. As a drawback, the performance is poorer.

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

Configuration is done using files (also called "resources"), which may reside locally or in an http location, and optionally may be created dynamicaly with a database query.

Some configuration resources are mandatory, such as the dictionaries or diameter peer files, as they are neede by Igor core. Others may be created for the use of custom handlers.

### Bootstrap: Searching the location of the configuration resources

The configuration process starts by accessing the bootstrap configuration file, which must be specified somehow (typicaly in a command line parameter). The
file has JSON format with two properties. The `rules` property defines where to look for files ("origin") based on the resource name. Those are checked
against a set of regular expressions with one group. The full location of the resource is the `origin` followed by the group matched in the regular expression.
The following example specifies that the `template_http.txt` file is retreived from http, the `radiusclients.database` configuration object is taken from a
database (more on this later) and the other objects are to be found as files in the same location as the boostrap file.

```
"rules": [
    {"nameRegex": "(template_http.txt)",            "origin": "http://localhost:8100/"},
    {"nameRegex": "(radiusclients.database)",       "origin": "database:accessNodes:AccessNodeId:Parameters"},
    {"nameRegex": "Gy/(.*)",                        "origin": ""},
    {"nameRegex": "(.*)",                           "origin": ""}
]
```

The origin for resources in files is relative to the path where the bootstrap file resides.
If the `origin` starts with `database` the syntax must be `database:<table-name>:<key-column-name>:<parameters-column-name>`. The result of the retrieval
will be a JSON file with the values in the `key-column-name` as properties and the corresponding values in `parameters-column-name` as values. For that
reason, this column MUST have JSON syntax.

A simple configuration where every resource is in the same directory as the bootstrap file would be

```
"rules": [
    {"nameRegex": "(.*)", "origin": ""}
]
```

When launching Igor, also an "instance name" must be specified. This is used to look for configuration resources. First, the name of the resource with the instance name is tried. If not found, then the raw name is tried. This way, multiple instances may share part of the configuration.

For origins pointing to a database, as explained above, the boostrap file may include a `db` property that specifies the url, driver and connections in the
pool.

### Dictionaries

The Diameter and Radius dictionaries are stored in resources named `radiusDictionary.json` and `diameterDictionary.json`. Those names are fixed. See the
provided files for hints on the syntax.

### Logging

Logging is configured in a resource called `log.json` (name is fixed). It will include two properties, one for the core logging and another for the logging to be used in the handlers. Uber zap is used as the loggging engine, and thus the corresponding configuration properties apply.

### Metrics

The resource `metrics.json` defines the bind address and port for exposing the Prometheus metrics.

### Radius configuration files

If the file `radiusServer.json` does not include a `bindAddress` property, the radius sever is not started and the rest of the radius configuration files are not read. In this resource, the basic parameters for radius are configured. Namely, the ports to lisen for authorization, accounting and CoA, and the origin ports to be used when acting as a radius client (`originPorts` property).

The other relevant configuration files are:
* `radiusClients.json` specifies the IP addresses from which radius requests may be received and the secret for each one of them. The IPAddress field may be
an IP address or a CIDR block, with syntax `IP mask/size`
* `radiusServers.json` specifies the upstream radius servers, grouped in radius groups. For each server, the origin ports may override what is specified in the global radius configuration, and the quarantine time an maximum errors in a row are specified. The Igor radius router accepts requests that may reference either a radius group or a single server (IP address) and explicit secret. In the latter case, the features that track the status of each server are not used
* `radiusHttpHandlers.json` specifies the URLs to invoke for each type of request, in case this kind of http handlers need to be invoked. Otherwise, local handling is used, using the handler function specified upon radius router creation

### Diameter configuration files

If the file `diameterServer.json` does not include a `bindAddress` property, the diameter server is not started and the rest of the diameter configuration files are not read.

The other relevant configuration files are:
* `diameterPeers.json` specifies the diameter peers. If the connection policy is `active`, the server will try to initiate the connection to the specified IP Address. If the connection policy is `passive` it will wait for connections to arrive, checking that the OriginNetwork matches.
* `diameterRoutes.json` specifies the action to take for each incoming message, based on the realm and applicationId. An `*` is used as wildcard. If `handlers` are specified, the requests are serialized and send to the specified URLs using http2, with random balancing. If `peers` are specified, one of the specified Diameter Peer is chosen to send the request to, using the specified policy, which may take the values `fixed` and `random`. Otherwise, that is, if no handler type is specified, the message is handled locally.

### Http router configuration

If a http router is spun, the configuration in `httpRouter.json` is taken into account. This will be the endpoint on which radius and diameter requests over http for the radius and diameter routers will be received. The router will handle or forward the requests to upstream radius and diameter servers. The purpose of the http router is to be able to instantiate radius and diameter clients that can be commanded using http and providing a way for external http handlers to generate radius and diameter requests to upstream servers.

### Http handler configuration

If Igor is launched as Http handler, the configuration in `httpHandler.json` is taken into account. This will be the endpoint on which radius and diameter requests will be received for the purpose of handling them. This module is provided as a sample and for testing, since the more common use case will be to handle this kinds of requests with a third party http2 server.

### Helpers for radius configuration

Igor provides some utilities for the development of handlers.

* `radiusChecks.json` define rules for classifying radius packets, for instance in order to determine whether they are session or service accounting, or whether they should be forwarded to upstream servers. An specific object at the disposal of handler developers, named `RadiusPacketChecks` is provided, and a configuration object with this parametrization may be created with the syntax that can be found in the examples. It contains a number of keys that specify the rules to be checked using binary operators and conditions of types `equals`, `present`, `notpresent`, `contains` or `matches` (a regular expression) for the specified radius attribute. The name of this file may be changed

* `radiusFilters.json` define rules for filering outgoing or incoming radius packets: removing attributes, adding attributes with a specific value, or explicitly copying a list of attributes. It is used by objects of type `AVPFilter` The name of this file may be changed.

### Standard configuration management

A `ConfigurationManager` object provides basic methods to manipulate configuration resources. It loads a bootstrap file and gets an instance name to be used when searching for objects, and retrieves them either as JSON object or just the raw bytes. Objects may be stored as local files, http URLs or in a database.

Specialized Configuration Managers include methods for manipulaing standard configuration objects for Policy (Radius and Diameter), or Http handlers. Those embed a `ConfigurationManager` which is used internally and also can be used externally to manipulate additional configuration objects.

Both the `PolicyConfigurationManager` and the `HttpHandlerConfigurationManager` take care of initializing the logging, dictionaries an the metrics upon instantiation. `PolicyConfigurationManager` offers specific methods for getting the standard configuration, such as `DiameterServerConf()` and a few others.

A production application will typically instantiate only one specialized Configuration Manager, using a single instance. For testing, though, multiple instances may be created. The instance marked as `default` upon instantiation will be the only one initializing loggers and dictionaries, which are shared among all instances.

### Custom configuration management

Given any type `T` that can be serialized to JSON, an object of type `ConfigObject[T any]` can be instantiated and manipulated with the facilities in the core package. Its `Update()` method, which takes a `ConfigurationManager` as parameter, will force the retrieval of the contents using the Igor configuration system. The `Get()` method will be used to get a copy of the contents. `Update()` may be called at any time to refresh the contents and do hot updating, without affecting the contents of the objects already copied using previous `Get()` invokations.

So, for instance, a configuration is defined and retrieved like this

```
var realms *core.ConfigObject[handler.RadiusUserFile]
realms = core.NewConfigObject[handler.RadiusUserFile]("realms.json")
if err = realms.Update(&ci.CM); err != nil {
    return fmt.Errorf("could not get realm configuration: %w", err)
}
```

A more specialized version of a configuration object is the `TemplatedConfigObject[T, P any]`, where `T` is the type of the template object, `P` is the type of the parameter object. When creating an object of this type, a text with a Go template that produces an objec of type `T` is passed, along with an instance of `map[string]P` that contains the parameters of the template for different values of a key. The produced object is a map from those keys to objects of type `P`. For instance, the parameters map may contain a set of values for different service names. `T` may contain the parametrization of a service, and the resulting object will be map where the configuration for each object is retrieved.

So, for instance, here the configuration object "planparameters" contains a map of plan names to plan parameters, and "basicProfiles.txt" is a template to produce a UserFile.

```
// Service configuration
var basicProfiles *core.TemplatedConfigObject[handler.RadiusUserFile, PlanTemplateParams]
basicProfiles = core.NewTemplatedConfigObject[handler.RadiusUserFile, PlanTemplateParams]("basicProfiles.txt", "planparameters")
if err = basicProfiles.Update(&ci.CM); err != nil {
    return fmt.Errorf("could not get basic profiles: %w", err)
}
```

## Logging

For logging core operations, simply retrieve a logger and use zap functions.

```
core.GetLogger().Debugf("received packet: %v", reqBuf[:packetSize])
```

For logging in handlers, where it is required that all the log entries appear together, the following pattern must be implemented

First, get and instance of the handler logger to use to invoke zap logging functions. Then, to do the final printing, invoke `WriteLog()` in a defered call

```
func EmptyDiameterHandler(request *core.DiameterMessage) (*core.DiameterMessage, error) {
	hl := core.NewHandlerLogger()
	l := hl.L

	defer func(l *core.HandlerLogger) {
		l.WriteLog()
	}(hl)

	l.Infof("%s", "Starting EmptyDiameterHandler")
	l.Infof("%s %s", "request", request)
```


