# Derivative Microservices

Essentially this repository contains a re-write of the Islandora microserives: houdini, homarus, hypercube, and FITS (a TODO).  It should be considered prototype-level quality.  The microservices use STOMP to communicate with ActiveMQ.  [AMQ-4710](https://issues.apache.org/jira/browse/AMQ-4710) is a long-standing bug impacting the reliability of STOMP clients, so use of these microserivces requires a [patched version of ActiveMQ](https://github.com/jhu-idc/idc-isle-buildkit/pull/89).

## Usage

```shell
$ ./derivative-ms -h
Usage of /Users/esm/go/bin/derivative-ms:
  -ack string
        STOMP acknowledgment mode, e.g. 'client' or 'auto' (default "client")
  -config string
        Path to handler configuration file
  -host string
        STOMP broker host name, e.g. 'islandora-idc.traefik.me' (default "localhost")
  -pass string
        STOMP broker password
  -port int
        STOMP broker port (default 61613)
  -queue string
        Queue to read messages from, e.g. 'islandora-connector-homarus' or 'ActiveMQ.DLQ'
  -user string
        STOMP broker user name
```

| Argument | Required | Default           | Description |
|---       |---       |---                |---
|ack       | yes      | `client`          | STOMP message acknowledgement mode |
|config    | no       | embedded config   | path to microservice handler configuration file |
|host      | yes      | `localhost`       | STOMP broker host name |
|port      | yes      | `61613`           | STOMP broker port |
|user      | no       | ""                | STOMP broker user name |
|pass      | no       | ""                | STOMP broker password |
|queue     | yes      | ""                | STOMP queue to listen to |


## Motivation

The rewrite comes down to the unpredictable scaling and behavior of the PHP-based Islandora microservices.  

Islandora microservices are serial: they process one message at a time from their respective queues until the queues are empty.  Aside from taking a long time to process a queue, a large ingest from one of the content administrators could create enough requests in the queue that their JWT tokens expire before the message has a chance to be processed.  A work-around is to create messages with JWTs that expire far into the future, but the real solution is to implement JWT renewal and scale up the microservices.

The Islandora microservices can scale in a couple of ways:
* a single microservice could process multiple messages concurrently
* _n_ instances of a microservice could be provisioned, each instance processing messages serially.

The former approach requires a potentially expensive cloud instance which may not always be busy.  The latter approach could be implemented on smaller compute instances, and _n_ could be raised or lowered based on load.

Alpaca is the component in the Islandora architecture that is responsible for handling messages and dispatching them to the PHP-based microservices.  It's based on Apache Karaf, and uses Camel to process messages.  Scaling _ought_ to work by creating multiple instances of the PHP-microservices in Docker, and instantiating multiple instances of their respective Camel contexts in Alpaca.  This works, kind of.  It's clear from the ActiveMQ console that some round-robining of requests occurs, spreading the load across the PHP microservices, but it doesn't behave as expected (e.g. one of the microservices will recieve the majority of the requests, and Alpaca will not immediately remove a message from the queue despite microservice instances being free, ready to work).

Since Karaf and Camel are based on old paradigms, impenatrable logic, and result in behaviors that are hard to understand, the microservices were re-written in Go and eliminate Karaf and Camel from the architecture.

## Architecture

Alpaca is not used in this architecture.  The microservices in this repository communicate directly with the message broker (ActiveMQ).  Reliably scaling them is as easy as starting another instance of the microservice, reading from the same queue.  The microservices compete for messages on the queue.  If the queue is deep, scale up by increasing the number of microservices.  If the queue is shallow, scale down.

The code for _all_ the microservices exists in this repository.  Each microservice is implemented as an instance of [`Handler`](https://github.com/jhu-idc/derivative-ms/blob/master/listener/listener.go#L51).  Basically handlers respond to messages based on their message destination (i.e. their ActiveMQ queue).  So the ImageMagick handler responds to the Houdini queue, and the FFMpegHandler responds to the Homarus queue, and so forth.  The Islandora mental model of the "Houdini microservice processes images" or "Homarus processes video" is maintained.

An instance of the microservice can only listen for messages on a single queue.  So while the command-line binary possesses the code necessary for handling any message from any queue, a specific instance will only handle messages from a single queue.  The only difference between an instance of the Houdini microserivce and the Homarus microservice will be the queue that they listen to.


## Message Handling

Islandora microservices are idempotent, so at-least-once messaging semantics are adequate.  If a duplicate message is received, the worst thing that happens is the generation of an identical derivative.  If a message is _lost_ or _rejected_, then a derivative (e.g. a thumbnail or service copy) will be missing from the object's page in Islandora.

If a [`Handler`](https://github.com/jhu-idc/derivative-ms/blob/master/listener/listener.go#L51) returns an error, then the message will be nacked.  Attempts to redeliver the message will be made over the next five minutes, in case the error was transient (or fixed).  However, if all redelivery attempts result in error, the message will go to the ActiveMQ dead letter queue (named `ActiveMQ.DLQ`), and no derivative will be generated.  The message is not strictly _lost_, as it is in the DLQ, but this microservice prototype does not provide any means to process messages in the DLQ.  Effectively the DLQ provides a mechanism for observing failures, but doesn't provide means to re-process those messages.

## TODOs

There are a number of TODOs, but the prototype is mature enough for demonstration purposes.

* Debugging output: it would be nice to put a microservice in debug mode and capture `stderr`.  The microservice would have a micro-frontend that would allow viewing of the debug output.
* Dead letter queue processing: Re-processing messages from the DLQ would be nice, but the best that we may be able to do is output a log message to `stdout`, surfacing messages to graylog, for example.
* FITS microservice: the FITS microservice needs to be implemented.
* JWT refresh: it would be nice to implement [JWT refresh](https://auth0.com/blog/refresh-tokens-what-are-they-and-when-to-use-them/).  To my knowledge, this is not supported by Drupal, so in effect a "refresh" would mean having Drupal issue a new key to the microservice, which is basically a stand-in for Basic Auth (you'd have to use Basic Auth to get a JWT, so why not just use Basic Auth when communicating with Drupal?).  So at this point the best defense against expiring keys is to either scale up the microservices to insure messages are processed within the JWT expiry window, or simply just use Basic Auth when communicating with Drupal, and skip the use of keys.  As far as I know, none of the claims provided in the JWT are used by microserivces.
* It is possible for a message to _not_ be handled by any handler.  This results in the message being acked anyway.  Bug?
* Test coverage: there are no tests (eep)
* Tesseract and pdftotext handlers are not well-exercised and may contain bugs
* Debugging statements and files (e.g. capture of cli stderr) abound



