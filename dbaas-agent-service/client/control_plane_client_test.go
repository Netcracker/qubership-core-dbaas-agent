package client

import (
	"context"
	"errors"
	"net/http"
	"testing"

	_ "github.com/netcracker/qubership-core-dbaas-agent/dbaas-agent-service/v2/config"
	"github.com/stretchr/testify/suite"
)

type ControlPlaneClientTestSuite struct {
	suite.Suite
}

func TestControlPlaneClientTestSuite(t *testing.T) {
	suite.Run(t, new(ControlPlaneClientTestSuite))
}

func (suite *ControlPlaneClientTestSuite) TestGetCompositeStructure_ControlPlaneReturnError() {
	restClient := getMockRestClient("", http.StatusInternalServerError, errors.New("unknown error"))
	controlPlaneClient := NewControlPlaneClient(restClient)
	_, err := controlPlaneClient.GetCompositeStructure(context.Background())
	suite.ErrorContains(err, "error getting composite structure from control-plane: unknown error")
}

func (suite *ControlPlaneClientTestSuite) TestGetCompositeStructure_ControlPlaneReturnUnexpectedStatus() {
	restClient := getMockRestClient("unknown error", http.StatusInternalServerError, nil)
	controlPlaneClient := NewControlPlaneClient(restClient)
	_, err := controlPlaneClient.GetCompositeStructure(context.Background())
	suite.ErrorContains(err, "control-plane returned unexpected code. Expected 200 but got 500, response: unknown error")
}

func (suite *ControlPlaneClientTestSuite) TestGetCompositeStructure_BadCompositeStructure() {
	restClient := getMockRestClient("bad_composite_structure", http.StatusOK, nil)
	controlPlaneClient := NewControlPlaneClient(restClient)
	compositeStructure, err := controlPlaneClient.GetCompositeStructure(context.Background())
	suite.Nil(compositeStructure)
	suite.ErrorContains(err, "failed to parse composite structure response body")
}

func (suite *ControlPlaneClientTestSuite) TestGetCompositeStructure_GotCompositeStructure() {
	restClient := getMockRestClient(serialize(CompositeStructure{Baseline: "ns-1", Satellites: []string{"ns-2"}}), http.StatusOK, nil)
	controlPlaneClient := NewControlPlaneClient(restClient)
	compositeStructure, err := controlPlaneClient.GetCompositeStructure(context.Background())
	suite.Nil(err)
	suite.Equal("ns-1", compositeStructure.Baseline)
	suite.Equal(1, len(compositeStructure.Satellites))
	suite.Equal("ns-2", compositeStructure.Satellites[0])
	suite.Equal(compositeStructure, controlPlaneClient.GetCompositeStructureFromCache())
}
