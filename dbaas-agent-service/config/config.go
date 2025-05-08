package config

import (
	fib_security "github.com/netcracker/qubership-core-lib-go-fiber-server-utils/v2/security"
	"github.com/netcracker/qubership-core-lib-go/v3/security"
	"github.com/netcracker/qubership-core-lib-go/v3/serviceloader"
)

func init() {
	serviceloader.Register(1, &fib_security.DummyFiberServerSecurityMiddleware{})
	serviceloader.Register(1, &security.DummyToken{})
	serviceloader.Register(1, &ClassifierServiceName{})
}
