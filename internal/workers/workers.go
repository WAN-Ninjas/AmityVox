// Package workers implements background job processing for tasks such as embed
// unfurling, media transcoding, expired session cleanup, and federation message
// delivery retry. Workers consume jobs from NATS JetStream queues.
// This package will be fully implemented in Phase 2.
package workers
