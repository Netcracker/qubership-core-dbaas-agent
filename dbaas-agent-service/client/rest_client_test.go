package client

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/netcracker/qubership-core-dbaas-agent/dbaas-agent-service/v2/internal/m2mtest"
	"github.com/netcracker/qubership-core-dbaas-agent/dbaas-agent-service/v2/testutils"
	"net/http"
	"testing"

	_ "github.com/netcracker/qubership-core-dbaas-agent/dbaas-agent-service/v2/config"
	"github.com/netcracker/qubership-core-lib-go/v3/configloader"
	constants "github.com/netcracker/qubership-core-lib-go/v3/const"
	"github.com/netcracker/qubership-core-lib-go/v3/security"
	"github.com/stretchr/testify/assert"
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
	restClient.Sender = testutils.FakeSender{Err: errors.New("can't get token")}
	_, _, err := restClient.DoRequest(context.Background(), http.MethodGet, "/cp", nil, logger)
	suite.ErrorContains(err, "can't get token")
}

func (suite *RestClientTestSuite) TestDoRequest_ErrorOnSending() {
	restClient := NewRestClient("")
	restClient.Sender = testutils.FakeSender{}
	_, _, err := restClient.DoRequest(context.Background(), http.MethodGet, "/cp", nil, logger)
	suite.ErrorContains(err, "missing port in address")
}

func (suite *RestClientTestSuite) TestConstructRequest_AddSlash() {
	restClient := NewRestClient("")
	restClient.Sender = testutils.FakeSender{}
	request, err := restClient.constructRequest(context.Background(), http.MethodGet, restClient.fullUrl("/cp"), "token", nil, logger)
	suite.Nil(err)
	suite.Equal("/cp", string(request.RequestURI()))

	request, err = restClient.constructRequest(context.Background(), http.MethodGet, restClient.fullUrl("cp"), "token", nil, logger)
	suite.Nil(err)
	suite.Equal("/cp", string(request.RequestURI()))
}

func (suite *RestClientTestSuite) TestConstructRequest_WithBody() {
	restClient := NewRestClient("")
	restClient.Sender = testutils.FakeSender{}
	request, err := restClient.constructRequest(context.Background(), http.MethodGet, restClient.fullUrl("/cp"), "token", []byte("somebody"), logger)
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

func (suite *RestClientTestSuite) TestDoRequest_Resend() {
	restClient := NewRestClient("")
	restClient.Sender = testutils.FakeSender{Times: 2}
	calls := 0
	restClient.Do = func(request *fasthttp.Request, response *fasthttp.Response) error {
		calls++
		if calls == 1 {
			response.SetBodyString("unauthorized")
			response.SetStatusCode(http.StatusUnauthorized)
			return nil
		}
		response.SetBodyString("body")
		response.SetStatusCode(http.StatusOK)
		return nil
	}
	resBody, code, err := restClient.DoRequest(context.Background(), http.MethodGet, "/cp", nil, logger)
	suite.Nil(err)
	suite.Equal(2, calls)
	suite.Equal(http.StatusOK, code)
	suite.Equal("body", string(resBody))
}

// m2mRestClient returns a client with the real M2M sender for the given mode; Do records the Authorization
// headers and the bodies, and answers 200 only to the accepted tokens.
func m2mRestClient(t *testing.T, mode security.M2MAuthMode, accepted ...string) (*RestClient, *[]string) {
	restClient := NewRestClient("http://dbaas-test")
	restClient.Sender = m2mtest.NewSender(t, mode)
	var received []string
	restClient.Do = func(request *fasthttp.Request, response *fasthttp.Response) error {
		auth := string(request.Header.Peek("Authorization"))
		received = append(received, auth+"|"+string(request.Body()))
		for _, token := range accepted {
			if auth == "Bearer "+token {
				response.SetBodyString("ok")
				response.SetStatusCode(http.StatusOK)
				return nil
			}
		}
		response.SetBodyString("denied")
		response.SetStatusCode(http.StatusUnauthorized)
		return nil
	}
	return restClient, &received
}

func TestDoRequest_K8sModeSendsOnlyK8sToken(t *testing.T) {
	restClient, received := m2mRestClient(t, security.M2MAuthModeK8s, m2mtest.K8sToken)
	body, code, err := restClient.DoRequest(context.Background(), http.MethodPost, "/cp", []byte("payload"), logger)
	assert.NoError(t, err)
	assert.Equal(t, http.StatusOK, code)
	assert.Equal(t, "ok", string(body))
	assert.Equal(t, []string{"Bearer " + m2mtest.K8sToken + "|payload"}, *received)
	assert.Zero(t, m2mtest.LegacyCalls.Load())
}

func TestDoRequest_K8sModeNoFallbackOn401(t *testing.T) {
	restClient, received := m2mRestClient(t, security.M2MAuthModeK8s, m2mtest.LegacyToken)
	body, code, err := restClient.DoRequest(context.Background(), http.MethodPost, "/cp", []byte("payload"), logger)
	assert.NoError(t, err)
	assert.Equal(t, http.StatusUnauthorized, code)
	assert.Equal(t, "denied", string(body))
	assert.Len(t, *received, 1)
	assert.Zero(t, m2mtest.LegacyCalls.Load())
}

func TestDoRequest_K8sModeTokenError(t *testing.T) {
	restClient, received := m2mRestClient(t, security.M2MAuthModeK8s, m2mtest.K8sToken)
	tokenErr := errors.New("no k8s token")
	m2mtest.SetK8sTokenError(tokenErr)
	_, _, err := restClient.DoRequest(context.Background(), http.MethodGet, "/cp", nil, logger)
	assert.ErrorIs(t, err, tokenErr)
	assert.Empty(t, *received)
}

func TestDoRequest_HybridModeSendsK8sToken(t *testing.T) {
	restClient, received := m2mRestClient(t, security.M2MAuthModeHybrid, m2mtest.K8sToken)
	_, code, err := restClient.DoRequest(context.Background(), http.MethodPost, "/cp", []byte("payload"), logger)
	assert.NoError(t, err)
	assert.Equal(t, http.StatusOK, code)
	assert.Equal(t, []string{"Bearer " + m2mtest.K8sToken + "|payload"}, *received)
	assert.Zero(t, m2mtest.LegacyCalls.Load())
}

func TestDoRequest_HybridModeResendsWithLegacyTokenOn401(t *testing.T) {
	restClient, received := m2mRestClient(t, security.M2MAuthModeHybrid, m2mtest.LegacyToken)
	body, code, err := restClient.DoRequest(context.Background(), http.MethodPost, "/cp", []byte("payload"), logger)
	assert.NoError(t, err)
	assert.Equal(t, http.StatusOK, code)
	assert.Equal(t, "ok", string(body), "the last response must be returned")
	assert.Equal(t, []string{
		"Bearer " + m2mtest.K8sToken + "|payload",
		"Bearer " + m2mtest.LegacyToken + "|payload",
	}, *received)

	// the sender remembers that the target only accepts the legacy token
	_, code, err = restClient.DoRequest(context.Background(), http.MethodPost, "/cp", []byte("payload"), logger)
	assert.NoError(t, err)
	assert.Equal(t, http.StatusOK, code)
	assert.Len(t, *received, 3)
	assert.Equal(t, "Bearer "+m2mtest.LegacyToken+"|payload", (*received)[2])
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
	restClient.Sender = testutils.FakeSender{}
	restClient.Do = func(request *fasthttp.Request, response *fasthttp.Response) error {
		response.SetBodyString(respBody)
		response.SetStatusCode(httpCode)
		return err
	}
	return restClient
}
