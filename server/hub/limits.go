package hub

// Limits and origin rules for a publicly reachable server.
var (
	// OriginPatterns lists the browser origins allowed to open a websocket
	// (same-origin requests are always allowed). Clients without an Origin
	// header, such as the Python load test, are also allowed.
	OriginPatterns = []string{"localhost:*", "127.0.0.1:*"}

	MaxMessageBytes   int64 = 8192
	MaxRooms                = 200
	MaxClientsPerRoom       = 100
	MaxRoomNameLen          = 64
)

// admit reports whether a connection may use this room name: existing rooms
// are always fine, new ones only while under the room cap.
func (h *Hub) admit(name string) bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	if _, ok := h.rooms[name]; ok {
		return true
	}
	return len(h.rooms) < MaxRooms
}
