package testutils

import (
	"context"

	"github.com/netcracker/qubership-core-lib-go/v3/security/rest"
)

// FakeSender is a client.TokenSender for tests: it sends with a fixed token, or fails with Err.
type FakeSender struct{ Err error }

func (f FakeSender) Send(ctx context.Context, targetUrl string, send rest.SendFunc) error {
	if f.Err != nil {
		return f.Err
	}
	_, err := send("token")
	return err
}
