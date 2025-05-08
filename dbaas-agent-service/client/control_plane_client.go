package client

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	"github.com/netcracker/qubership-core-lib-go/v3/logging"
	"github.com/valyala/fasthttp"
)

const compositeStructureUrl = "/api/v3/control-plane/composite-platform/namespaces"

var logger logging.Logger

func init() {
	logger = logging.GetLogger("control-plane-client")
}

type ControlPlaneClient struct {
	restClient              *RestClient
	compositeStructureCache *CompositeStructure
}

func NewControlPlaneClient(restClient *RestClient) *ControlPlaneClient {
	return &ControlPlaneClient{restClient: restClient}
}

type CompositeStructure struct {
	Baseline   string   `json:"baseline"`
	Satellites []string `json:"satellites"`
}

func (cpClient *ControlPlaneClient) GetCompositeStructure(ctx context.Context) (*CompositeStructure, error) {
	logger.DebugC(ctx, "request to get composite structure from control-plane")
	resp, httpCode, err := cpClient.restClient.DoRequest(ctx, fasthttp.MethodGet, compositeStructureUrl, nil, logger)
	logger.InfoC(ctx, "composite structure response: %s, httpCode %d, error: %+v", resp, httpCode, err)
	if err != nil {
		return nil, errors.New(fmt.Sprintf("error getting composite structure from control-plane: %s", err))
	}
	if httpCode != http.StatusOK {
		return nil, errors.New(fmt.Sprintf("control-plane returned unexpected code. Expected 200 but got %d, response: %s", httpCode, resp))
	}
	compositeStructure := new(CompositeStructure)
	if err := json.Unmarshal(resp, compositeStructure); err != nil {
		return nil, errors.New("failed to parse composite structure response body" + err.Error())
	}
	cpClient.compositeStructureCache = compositeStructure
	return compositeStructure, nil
}

func (cpClient *ControlPlaneClient) GetCompositeStructureFromCache() *CompositeStructure {
	return cpClient.compositeStructureCache
}
