package testutils

import (
	"context"

	"github.com/netcracker/qubership-core-lib-go/v3/security/rest"
)

// FakeSender is a client.TokenSender for tests: it sends with a fixed token, or fails with Err.
// Times is the number of sends (default 1); more than one simulates the resend after a 401.
type FakeSender struct {
	Err   error
	Times int
}

func (f FakeSender) Send(ctx context.Context, targetUrl string, send rest.SendFunc) error {
	if f.Err != nil {
		return f.Err
	}
	var err error
	for i := 0; i < max(f.Times, 1); i++ {
		_, err = send("token")
	}
	return err
}
