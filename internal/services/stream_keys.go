package services

// StreamKey is used to key console/stats streaming maps.
// We use the string Server.ID (UUID) to avoid needing numeric IDs.
type StreamKey string
