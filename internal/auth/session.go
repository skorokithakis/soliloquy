package auth

import "time"

// SessionLifetime is how long a session lasts after the user's last visit.
const SessionLifetime = 365 * 24 * time.Hour
