package internal

// Protocol Handlers register with the gossip service to receive incoming
// gossip messages on a specific channel.

// Protocol Handlers also typically send gossip messages periodically,
// and therefore are often a service.

type ProtocolHandler interface {
}
