# DogeNet

This project is hosted on [radicle.xyz](https://radicle.xyz) at [rad:z213iZozZ4wgoGntPGkMfe1jmX4RY](https://app.radicle.xyz/nodes/ash.radicle.garden/z213iZozZ4wgoGntPGkMfe1jmX4RY)

## DogeNet Service

DogeNet is a service that implements the DogeBox Gossip Protocol, which is a
multi-channel protocol for communicating among DogeBoxes.

It is a foundational piece for future protocols (ComPoS/Sakura)	

## Node Database

DogeNet builds a database of active DogeBox nodes, which is used to find
peer nodes to connect to. DogeNet maintains a connection to at least 6
peers at all times (more if running *novel* protocol-handlers that require
connecting to other participating nodes.)

The node database is also used to populate the DogeMap *pup*[^1].

## Core Nodes

DogeNet includes a background *crawler* service to discover Core Nodes
for display in the DogeMap *pup*[^1]. This service is optional.

When DogeNet is configured with a local Core Node ip-address, it also
*scrapes* known Core Nodes periodically from the local Core Node.

This is likely sufficient to approximaely map out the active Core Nodes
over time, without placing any additional load on the Core network.

## Protocol Handlers

DogeNet exposes a local UNIX-domain socket for Protocol Handlers to connect
to DogeNet and send/receive messages on any channels they're interested in.

Channels are assigned to different protocols; for example, DogeNet nodes
announce their presence and public address on the `Node` channel; DogeBoxes
can optionally announce an identity profile on the `Iden` channel, for
display on the DogeMap and for use in future social *pups*[^1].

This facility is currently used by the Identity Protocol-Handler:
[rad:z4FoA61FxfXyXpfDovtPKQQfiWJWH](https://app.radicle.xyz/nodes/ash.radicle.garden/z4FoA61FxfXyXpfDovtPKQQfiWJWH)


[^1]: a **pup** is a small application package that can be installed on the DogeBox.
Pups run inside a security sandbox; they can access local services on the box
when granted permission to do so.
