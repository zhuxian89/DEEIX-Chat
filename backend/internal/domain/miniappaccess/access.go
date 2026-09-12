package miniappaccess

import "context"

// Decision is additional access policy, never a replacement for authentication.
type Decision struct {
	MiniApp        bool
	Reauthenticate bool
	Unlocked       bool
}

type Reader interface {
	Access(context.Context, uint, string) (Decision, error)
}

type Registrar interface {
	Register(context.Context, uint, string, string, string) error
}
