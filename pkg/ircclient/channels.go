package ircclient

// Channel-related methods are implemented directly on ircClient in connection.go:
// - Join(channel)
// - Part(channel)
// - JoinedChannels()
// - SendMessage(target, message)
// - SendNotice(target, message)
// - SendRaw(message)
//
// This file exists as a placeholder for future channel management features
// such as channel mode tracking, topic tracking, and user list management.
