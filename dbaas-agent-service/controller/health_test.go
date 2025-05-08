package controller

import (
	"encoding/json"
	"io"
	"net/http"

	"github.com/netcracker/qubership-core-dbaas-agent/dbaas-agent-service/v2/service"
	"github.com/netcracker/qubership-core-dbaas-agent/dbaas-agent-service/v2/testutils"
	fiberserver "github.com/netcracker/qubership-core-lib-go-fiber-server-utils/v2"
	"github.com/stretchr/testify/assert"
)

var (
	testToken          = "test-token"
	testTokenExpiresIn = 300
)

func (suite *TestSuite) TestController_HandleGetHealth() {
	testRespBodyBytes := createTestHealthResponse()
	testutils.AddHandler(testutils.Contains("/health"), func(rw http.ResponseWriter, req *http.Request) {
		_, _ = rw.Write(testRespBodyBytes)
	})

	app, err := fiberserver.New().Process()
	assert.Nil(suite.T(), err)
	app.Get("/health", suite.controller.HandleGetHealth)
	req, _ := http.NewRequest(http.MethodGet, "/health", nil)
	response, err := app.Test(req, -1)
	assert.Nil(suite.T(), err)
	respBody, err := io.ReadAll(response.Body)
	assert.Nil(suite.T(), err)

	assert.Equal(suite.T(), http.StatusOK, response.StatusCode)
	assert.Equal(suite.T(), testRespBodyBytes, respBody)
}

func (suite *TestSuite) TestController_HandleProbes() {
	app, err := fiberserver.New().Process()
	assert.Nil(suite.T(), err)
	app.Get("/probes/live", suite.controller.HandleProbes)
	req, _ := http.NewRequest(http.MethodGet, "/probes/live", nil)
	response, err := app.Test(req, -1)
	assert.Nil(suite.T(), err)
	assert.Equal(suite.T(), http.StatusOK, response.StatusCode)
}

func (suite *TestSuite) TestController_HandleGetHealthNegative() {
	testRespBodyBytes := createTestHealthResponse()
	testutils.AddHandler(testutils.Contains("/health"), func(rw http.ResponseWriter, req *http.Request) {
		_, _ = rw.Write(testRespBodyBytes)
	})
	testCreds := service.NewBasicCreds("test-user", []byte("test-pass"))
	forwarder := service.NewForwarder("http://fake-url", testCreds, testutils.GetMockServerClient())
	controller := NewController(nil, forwarder)

	app, err := fiberserver.New().Process()
	assert.Nil(suite.T(), err)
	app.Get("/health", controller.HandleGetHealth)
	req, _ := http.NewRequest(http.MethodGet, "/health", nil)
	response, err := app.Test(req, -1)
	assert.Nil(suite.T(), err)

	assert.True(suite.T(), response.StatusCode > 400)
}

func createTestHealthResponse() []byte {
	details := map[string]interface{}{
		"mongoAdapterHealth": map[string]interface{}{
			"status": "UP",
		},
	}
	testRespBody := map[string]interface{}{
		"status":  "UP",
		"details": details,
	}
	testRespBodyBytes, _ := json.Marshal(testRespBody)
	return testRespBodyBytes
}
