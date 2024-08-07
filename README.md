# DogeNet

This project is hosted on [radicle.xyz](https://radicle.xyz) at [rad:z213iZozZ4wgoGntPGkMfe1jmX4RY](https://app.radicle.xyz/nodes/ash.radicle.garden/z213iZozZ4wgoGntPGkMfe1jmX4RY)

## DogeNet Service

DogeNet is a service that implements the DogeBox Gossip Protocol, which is a
multi-channel protocol for communicating among DogeBoxes.

DogeNet builds a database of active DogeBox nodes, which is also used to populate the DogeMap *pup*.

It is the foundational piece for future protocols (ComPoS/Sakura)	

DogeNet exposes a local UNIX-domain socket so Protocol Handlers can connect to DogeNet
and send/receive messages on any channels they're interested in.

This facility is currently used by the Identity Protocol-Handler:
[rad:z4FoA61FxfXyXpfDovtPKQQfiWJWH](https://app.radicle.xyz/nodes/ash.radicle.garden/z4FoA61FxfXyXpfDovtPKQQfiWJWH)
