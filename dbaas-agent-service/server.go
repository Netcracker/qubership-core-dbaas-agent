package main

import (
	_ "github.com/netcracker/qubership-core-dbaas-agent/dbaas-agent-service/v2/config"
	service "github.com/netcracker/qubership-core-dbaas-agent/dbaas-agent-service/v2/lib"
)

func main() {
	service.RunServer()
}
