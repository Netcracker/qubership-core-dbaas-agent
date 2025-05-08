package client

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"testing"

	_ "github.com/netcracker/qubership-core-dbaas-agent/dbaas-agent-service/v2/config"
	"github.com/netcracker/qubership-core-lib-go/v3/configloader"
	constants "github.com/netcracker/qubership-core-lib-go/v3/const"
	"github.com/stretchr/testify/suite"
	"github.com/valyala/fasthttp"
)

type RestClientTestSuite struct {
	suite.Suite
}

func TestRestClientTestSuite(t *testing.T) {
	suite.Run(t, new(RestClientTestSuite))
}

func (suite *RestClientTestSuite) TestGetInternalGatewayUrl_GetDefaultGatewayUrl() {
	configloader.InitWithSourcesArray([]*configloader.PropertySource{configloader.EnvPropertySource()})
	gatewayUrl := GetInternalGatewayUrl()
	suite.Equal(constants.DefaultHttpGatewayUrl, gatewayUrl)
}

func (suite *RestClientTestSuite) TestDoRequest_ErrorOnGettingToken() {
	restClient := NewRestClient("")
	restClient.GetToken = func(ctx context.Context) (string, error) {
		return "", errors.New("can't get token")
	}
	_, _, err := restClient.DoRequest(context.Background(), http.MethodGet, "/cp", nil, logger)
	suite.ErrorContains(err, "can't get token")
}

func (suite *RestClientTestSuite) TestDoRequest_ErrorOnSending() {
	restClient := NewRestClient("")
	restClient.GetToken = func(ctx context.Context) (string, error) {
		return "token", nil
	}
	_, _, err := restClient.DoRequest(context.Background(), http.MethodGet, "/cp", nil, logger)
	suite.ErrorContains(err, "missing port in address")
}

func (suite *RestClientTestSuite) TestConstructRequest_AddSlash() {
	restClient := NewRestClient("")
	restClient.GetToken = func(ctx context.Context) (string, error) {
		return "", nil
	}
	request, err := restClient.constructRequest(context.Background(), http.MethodGet, "/cp", nil, logger)
	suite.Nil(err)
	suite.Equal("/cp", string(request.RequestURI()))

	request, err = restClient.constructRequest(context.Background(), http.MethodGet, "cp", nil, logger)
	suite.Nil(err)
	suite.Equal("/cp", string(request.RequestURI()))
}

func (suite *RestClientTestSuite) TestConstructRequest_WithBody() {
	restClient := NewRestClient("")
	restClient.GetToken = func(ctx context.Context) (string, error) {
		return "", nil
	}
	request, err := restClient.constructRequest(context.Background(), http.MethodGet, "/cp", []byte("somebody"), logger)
	suite.Nil(err)
	suite.Equal("somebody", string(request.Body()))
}

func (suite *RestClientTestSuite) TestDoRequest_GetResponse() {
	restClient := getMockRestClient(serialize(CompositeStructure{Baseline: "ns-1"}), http.StatusOK, nil)
	resBody, code, err := restClient.DoRequest(context.Background(), http.MethodGet, "/cp", nil, logger)
	suite.Nil(err)
	suite.Equal(http.StatusOK, code)
	suite.Equal(http.StatusOK, code)
	suite.Equal(serialize(CompositeStructure{Baseline: "ns-1"}), string(resBody))
}

func serialize(obj interface{}) string {
	bytes, err := json.Marshal(obj)
	if err != nil {
		panic(err)
	}
	return string(bytes)
}

func getMockRestClient(respBody string, httpCode int, err error) *RestClient {
	restClient := NewRestClient("")
	restClient.GetToken = func(ctx context.Context) (string, error) {
		return "", nil
	}
	restClient.Do = func(request *fasthttp.Request, response *fasthttp.Response) error {
		response.SetBodyString(respBody)
		response.SetStatusCode(httpCode)
		return err
	}
	return restClient
}
