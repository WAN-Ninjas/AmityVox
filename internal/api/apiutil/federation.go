package apiutil

import "net/http"

// FederationProxy forwards mutations on federated guilds to their home instance.
// The SyncService in internal/federation satisfies this interface.
type FederationProxy interface {
	// ProxyToHomeInstance checks if the guild is federated (instanceID is non-nil)
	// and forwards the management request to the home instance. Returns true if
	// the request was forwarded (the handler should return immediately), or false
	// if the guild is local (the handler should continue with local logic).
	ProxyToHomeInstance(
		w http.ResponseWriter,
		r *http.Request,
		guildID string,
		instanceID *string,
		action string,
		userID string,
		data interface{},
	) bool

	// ProxyReadGuildMembers checks if the guild is federated and, if so,
	// fetches the member list from the home instance. Returns true if proxied
	// (handler should return), false if local (handler continues).
	ProxyReadGuildMembers(w http.ResponseWriter, r *http.Request, guildID string) bool

	// ProxyReadChannelMessages checks if the channel belongs to a federated
	// guild and, if so, fetches messages from the home instance. Returns true
	// if proxied (handler should return), false if local (handler continues).
	ProxyReadChannelMessages(w http.ResponseWriter, r *http.Request, channelID string) bool
}
