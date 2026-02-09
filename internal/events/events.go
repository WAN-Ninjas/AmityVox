// Package events implements the internal event bus using NATS pub/sub. REST API
// handlers publish events to NATS subjects, and the WebSocket gateway subscribes
// to dispatch real-time updates to connected clients. NATS JetStream provides
// persistent streams for federation message queuing.
// This package will be fully implemented in Phase 2.
package events
