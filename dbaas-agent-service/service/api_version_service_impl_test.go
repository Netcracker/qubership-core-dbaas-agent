package service

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/netcracker/qubership-core-dbaas-agent/dbaas-agent-service/v2/testutils"
	"github.com/netcracker/qubership-core-lib-go-actuator-common/v2/apiversion"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
)

type TestSuite struct {
	suite.Suite
	dbaasResp []byte
}

func (suite *TestSuite) SetupSuite() {
	testutils.StartMockServer()
	data := map[string]interface{}{
		"specs": []interface{}{
			map[string]interface{}{
				"specRootUrl":     "/api",
				"major":           3,
				"minor":           14,
				"supportedMajors": []int{3},
			},
			map[string]interface{}{
				"specRootUrl":     "/api/bluegreen",
				"major":           1,
				"minor":           3,
				"supportedMajors": []int{1},
			},
			map[string]interface{}{
				"specRootUrl":     "/api/declarations",
				"major":           1,
				"minor":           0,
				"supportedMajors": []int{1},
			},
			map[string]interface{}{
				"specRootUrl":     "/api/composite",
				"major":           1,
				"minor":           0,
				"supportedMajors": []int{1},
			},
		},
	}
	testRespBody, _ := json.Marshal(data)
	suite.dbaasResp = testRespBody
}

func TestExampleTestSuite(t *testing.T) {
	suite.Run(t, new(TestSuite))
}

func (suite *TestSuite) TestGetApiVersion() {
	testCreds := NewBasicCreds("test-user", []byte("test-pass"))
	forwarder := NewForwarder(testutils.GetMockServerUrl(), testCreds, testutils.GetMockServerClient())
	config := apiversion.ApiVersionConfig{
		PathToApiVersionInfoFile: "../api-version-info.json",
	}
	apiVersionService, err := NewApiVersionService(config, forwarder)
	assert.Nil(suite.T(), err)
	testutils.AddHandler(testutils.Contains("/api-version"), func(rw http.ResponseWriter, req *http.Request) {
		rw.WriteHeader(http.StatusOK)
		_, _ = rw.Write(suite.dbaasResp)
	})
	resp, err := apiVersionService.GetApiVersion(context.Background())
	assert.Nil(suite.T(), err)
	for _, spec := range resp.Specs {
		assert.NotNil(suite.T(), spec.SpecRootUrl)
	}
}

func (suite *TestSuite) TestGetApiVersionJsonError() {
	config := apiversion.ApiVersionConfig{
		PathToApiVersionInfoFile: "../testutils/api-version-info-wrong.json",
	}
	testCreds := NewBasicCreds("test-user", []byte("test-pass"))
	forwarder := NewForwarder(testutils.GetMockServerUrl(), testCreds, testutils.GetMockServerClient())

	testutils.AddHandler(testutils.Contains("/api-version"), func(rw http.ResponseWriter, req *http.Request) {
		rw.WriteHeader(http.StatusOK)
		_, _ = rw.Write(suite.dbaasResp)
	})

	apiVersionService, err := NewApiVersionService(config, forwarder)
	assert.Nil(suite.T(), err)
	_, err = apiVersionService.GetApiVersion(context.Background())
	assert.NotNil(suite.T(), err)
	assert.Equal(suite.T(), err.Error(), "spec.Major field can not be empty")
}
