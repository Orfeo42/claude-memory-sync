package syncer

import "time"

type Config struct {
	ServerURL  string
	Token      string
	ClientID   string
	SlugPrefix string
	ClaudeDir  string
	StateDir   string
	Interval   time.Duration
}
