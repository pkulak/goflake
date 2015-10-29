# Goflake

Goflake is a distributed id generation service that tries its best to hand out
ids that don't look like they came from a distributed id generation service.

## Guarantees

* Keys start at zero.
* Every key is unique.
* Every key from the same node will be ascending.
* Keys from multiple nodes that are not ascending will only be out of order by
  at most one second
* No external dependencies (co-ordination is taken care of on-site with the
  fantastic [Hashicorp raft library](https://github.com/hashicorp/raft))

## Goals

* As small and as few gaps as possible. The more variable the load to a node,
  the more and larger the gaps.
* Coordination once per second per node (though this can go up for highly
  variable loads).

## Usage

Start a single node (just for testing; use at least three for fault tolerance):

    goflake --config goflake.toml

and use the HTTP API to get new IDs:

    bender:~ phil$ curl -D - http://localhost:8080/next
    HTTP/1.1 200 OK
    Date: Tue, 27 Oct 2015 23:50:16 GMT
    Content-Length: 2
    Content-Type: text/plain; charset=utf-8

    0

## Use in production

No. Don't do it. This has never been used in a production environment, and you
probably don't want to be the first. Feel free to play with it and send me pull
requests, though!
