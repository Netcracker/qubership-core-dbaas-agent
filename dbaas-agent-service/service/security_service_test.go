package service

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"testing"

	"github.com/netcracker/qubership-core-dbaas-agent/dbaas-agent-service/v2/client"
	"github.com/stretchr/testify/suite"
	"github.com/valyala/fasthttp"
)

type SecServiceTestSuite struct {
	suite.Suite
}

func TestSecServiceTestSuite(t *testing.T) {
	suite.Run(t, new(SecServiceTestSuite))
}

func (suite *SecServiceTestSuite) TestGetCompositeStructure_GetErrAndCacheEmpty() {
	restClient := suite.getMockRestClient("unknown server error", http.StatusInternalServerError)
	controlPlaneClient := client.NewControlPlaneClient(restClient)
	tenantManagerClient := client.NewTenantManagerClient(restClient)
	securityService := NewSecurityService("", true, "ns-1", controlPlaneClient, tenantManagerClient)
	err := securityService.CheckNamespaceFromClassifier(context.Background(), "ns-2")
	suite.ErrorContains(err, "DbaaS-agent namespace and namespace in classifier is different. It's allowed only for composite platform. "+
		"Could not get composite structure from control-plane: "+
		"control-plane returned unexpected code. Expected 200 but got 500, response: unknown server error")
}

func (suite *SecServiceTestSuite) TestCheckNamespaceInClassifier_GetErrAndCacheNotEmpty() {
	restClient := suite.getMockRestClient(serialize(client.CompositeStructure{Baseline: "ns-1"}), http.StatusOK)
	controlPlaneClient := client.NewControlPlaneClient(restClient)
	tenantManagerClient := client.NewTenantManagerClient(restClient)
	compositeStructure, err2 := controlPlaneClient.GetCompositeStructure(context.Background())
	suite.Nil(err2)
	suite.Equal("ns-1", compositeStructure.Baseline)
	restClient.Do = func(request *fasthttp.Request, response *fasthttp.Response) error {
		response.SetBodyString("unknown server error")
		response.SetStatusCode(http.StatusInternalServerError)
		return nil
	}

	securityService := NewSecurityService("", true, "ns-2", controlPlaneClient, tenantManagerClient)
	err := securityService.CheckNamespaceFromClassifier(context.Background(), "ns-2")
	suite.Nil(err)

	err = securityService.CheckNamespaceFromClassifier(context.Background(), "ns-3")
	suite.ErrorContains(err, "orbidden! DbaaS-agent namespace and namespace in classifier must be the same. "+
		"It's prohibited to create or get database from another namespace except composite platform")
}

func (suite *SecServiceTestSuite) TestCheckNamespaceInClassifier_NamespaceNotInCompositePlatform() {
	restClient := suite.getMockRestClient(serialize(client.CompositeStructure{Baseline: "ns-1"}), http.StatusOK)
	controlPlaneClient := client.NewControlPlaneClient(restClient)
	tenantManagerClient := client.NewTenantManagerClient(restClient)
	securityService := NewSecurityService("", true, "ns-2", controlPlaneClient, tenantManagerClient)
	err := securityService.CheckNamespaceFromClassifier(context.Background(), "ns-2")
	suite.Nil(err)

	err = securityService.CheckNamespaceFromClassifier(context.Background(), "ns-3")
	suite.ErrorContains(err, "forbidden! DbaaS-agent namespace and namespace in classifier must be the same. "+
		"It's prohibited to create or get database from another namespace except composite platform")

	restClient.Do = func(request *fasthttp.Request, response *fasthttp.Response) error {
		response.SetBodyString(serialize(client.CompositeStructure{Baseline: "ns-1", Satellites: []string{"ns-3"}}))
		response.SetStatusCode(http.StatusOK)
		return errors.New("abc")
	}

	err = securityService.CheckNamespaceFromClassifier(context.Background(), "ns-4")
	suite.ErrorContains(err, "forbidden! DbaaS-agent namespace and namespace in classifier must be the same. "+
		"It's prohibited to create or get database from another namespace except composite platform")
}

func (suite *SecServiceTestSuite) TestCheckNamespaceInClassifier_NamespaceInCompositePlatform() {
	restClient := suite.getMockRestClient(serialize(client.CompositeStructure{Baseline: "ns-1"}), http.StatusOK)
	controlPlaneClient := client.NewControlPlaneClient(restClient)
	tenantManagerClient := client.NewTenantManagerClient(restClient)
	securityService := NewSecurityService("", true, "ns-2", controlPlaneClient, tenantManagerClient)
	err := securityService.CheckNamespaceFromClassifier(context.Background(), "ns-2")
	suite.Nil(err)

	err = securityService.CheckNamespaceFromClassifier(context.Background(), "ns-1")
	suite.Nil(err)

	restClient.Do = func(request *fasthttp.Request, response *fasthttp.Response) error {
		response.SetBodyString(serialize(client.CompositeStructure{Baseline: "ns-1", Satellites: []string{"ns-3"}}))
		response.SetStatusCode(http.StatusOK)
		return nil
	}

	err = securityService.CheckNamespaceFromClassifier(context.Background(), "ns-3")
	suite.Nil(err)
}

func (suite *SecServiceTestSuite) TestCheckNamespaceInClassifier_IsolationIsDisabled() {
	controlPlaneClient := client.NewControlPlaneClient(nil)
	tenantManagerClient := client.NewTenantManagerClient(nil)
	securityService := NewSecurityService("", false, "ns-2", controlPlaneClient, tenantManagerClient)
	err := securityService.CheckNamespaceFromClassifier(context.Background(), "ns-1")
	suite.Nil(err)
}

func (suite *SecServiceTestSuite) TestCheckTenantIdExist_GetErrAndCacheEmpty() {
	restClient := suite.getMockRestClient("unknown server error", http.StatusInternalServerError)
	controlPlaneClient := client.NewControlPlaneClient(restClient)
	tenantManagerClient := client.NewTenantManagerClient(restClient)
	securityService := NewSecurityService("", true, "ns-1", controlPlaneClient, tenantManagerClient)
	err := securityService.CheckTenantIdExist(context.Background(), "test-tenant-id")
	suite.ErrorContains(err, "Could not get tenants list from tenant-manager: tenant-manager returned unexpected code. Expected 200 but got 500, response: unknown server error")
}

func (suite *SecServiceTestSuite) TestCheckTenantIdExist_GetErrAndCacheIsNotEmpty() {
	restClient := suite.getMockRestClient("unknown server error", http.StatusInternalServerError)
	controlPlaneClient := client.NewControlPlaneClient(restClient)
	tenantManagerClient := client.NewTenantManagerClient(restClient)
	securityService := NewSecurityService("", true, "ns-1", controlPlaneClient, tenantManagerClient)
	err := securityService.CheckTenantIdExist(context.Background(), "test-tenant-id")
	suite.ErrorContains(err, "Could not get tenants list from tenant-manager: tenant-manager returned unexpected code. Expected 200 but got 500, response: unknown server error")
}

func (suite *SecServiceTestSuite) TestCheckTenantIdExist_TenantExistInList() {
	restClient := suite.getMockRestClient(serialize([]client.Tenant{{TenantId: "test-tenant-id"}}), http.StatusOK)
	controlPlaneClient := client.NewControlPlaneClient(restClient)
	tenantManagerClient := client.NewTenantManagerClient(restClient)
	securityService := NewSecurityService("", false, "", controlPlaneClient, tenantManagerClient)
	err := securityService.CheckTenantIdExist(context.Background(), "test-tenant-id")
	suite.Nil(err)

	restClient.Do = func(request *fasthttp.Request, response *fasthttp.Response) error {
		response.SetBodyString(serialize([]client.Tenant{{TenantId: "test-tenant-id"}}))
		response.SetStatusCode(http.StatusOK)
		return nil
	}
}

func (suite *SecServiceTestSuite) TestCheckNamespaceIsolation_SameNamespace() {
	controlPlaneClient := client.NewControlPlaneClient(nil)
	tenantManagerClient := client.NewTenantManagerClient(nil)
	securityService := NewSecurityService("", true, "namespace", controlPlaneClient, tenantManagerClient)
	err := securityService.CheckNamespaceIsolation(context.Background(), "namespace")
	suite.Nil(err)
}

func (suite *SecServiceTestSuite) TestCheckNamespaceIsolation_DifferentNamespaces() {
	restClient := suite.getMockRestClient(serialize(nil), http.StatusOK)
	controlPlaneClient := client.NewControlPlaneClient(restClient)
	tenantManagerClient := client.NewTenantManagerClient(restClient)
	securityService := NewSecurityService("", true, "namespace", controlPlaneClient, tenantManagerClient)
	err := securityService.CheckNamespaceIsolation(context.Background(), "another_namespace")
	suite.ErrorContains(err, "forbidden! DbaaS-agent namespace and namespace from path must be the same. It's prohibited to create or get database from another namespace except composite platform")
}

func (suite *SecServiceTestSuite) TestCheckNamespaceIsolation_DifferentNamespacesInCompositePlatform() {
	restClient := suite.getMockRestClient(serialize(client.CompositeStructure{Baseline: "another_namespace"}), http.StatusOK)
	controlPlaneClient := client.NewControlPlaneClient(restClient)
	tenantManagerClient := client.NewTenantManagerClient(restClient)
	securityService := NewSecurityService("", true, "namespace", controlPlaneClient, tenantManagerClient)
	err := securityService.CheckNamespaceIsolation(context.Background(), "another_namespace")
	suite.Nil(err)
	err = securityService.CheckNamespaceIsolation(context.Background(), "namespace")
	suite.Nil(err)

	restClient.Do = func(request *fasthttp.Request, response *fasthttp.Response) error {
		response.SetBodyString(serialize(client.CompositeStructure{Baseline: "another_namespace", Satellites: []string{"ns-3"}}))
		response.SetStatusCode(http.StatusOK)
		return nil
	}

	err = securityService.CheckNamespaceFromClassifier(context.Background(), "ns-3")
	suite.Nil(err)
}

func (suite *SecServiceTestSuite) TestValidateToken_BadToken() {
	controlPlaneClient := client.NewControlPlaneClient(nil)
	tenantManagerClient := client.NewTenantManagerClient(nil)
	securityService := NewSecurityService("", false, "", controlPlaneClient, tenantManagerClient)
	token, err := securityService.ValidateToken(context.Background(), "bad_token")
	suite.Nil(token)
	suite.Nil(err)
}

func (suite *SecServiceTestSuite) TestCheckTenantId_BodyNil() {
	controlPlaneClient := client.NewControlPlaneClient(nil)
	tenantManagerClient := client.NewTenantManagerClient(nil)
	securityService := NewSecurityService("", false, "", controlPlaneClient, tenantManagerClient)
	err := securityService.CheckTenantId(context.Background(), nil, "")
	suite.Nil(err)
}

func (suite *SecServiceTestSuite) TestCheckTenantId_EmptyBody() {
	controlPlaneClient := client.NewControlPlaneClient(nil)
	tenantManagerClient := client.NewTenantManagerClient(nil)
	securityService := NewSecurityService("", false, "", controlPlaneClient, tenantManagerClient)
	body := map[string]interface{}{}
	err := securityService.CheckTenantId(context.Background(), body, "")
	suite.Nil(err)
}

func (suite *SecServiceTestSuite) TestCheckTenantId_BodyWithBadClassifier() {
	controlPlaneClient := client.NewControlPlaneClient(nil)
	tenantManagerClient := client.NewTenantManagerClient(nil)
	securityService := NewSecurityService("", false, "", controlPlaneClient, tenantManagerClient)
	body := map[string]interface{}{
		"classifier": map[string]interface{}{"tenantId": "id"},
	}
	err := securityService.CheckTenantId(context.Background(), body, "")
	suite.ErrorContains(err, "tenantIds in classifier and header don't match")
}

func (suite *SecServiceTestSuite) TestCheckTenantId_BodyWithBadTenantId() {
	controlPlaneClient := client.NewControlPlaneClient(nil)
	tenantManagerClient := client.NewTenantManagerClient(nil)
	securityService := NewSecurityService("", false, "", controlPlaneClient, tenantManagerClient)
	body := map[string]interface{}{
		"tenantId": "id",
	}
	err := securityService.CheckTenantId(context.Background(), body, "")
	suite.ErrorContains(err, "tenantIds in classifier and header don't match")
}

func (suite *SecServiceTestSuite) TestCheckTenantId_BodyWithForbiddenTenantId() {
	restClient := suite.getMockRestClient(serialize([]client.Tenant{{TenantId: "test-tenant-id"}}), http.StatusOK)
	controlPlaneClient := client.NewControlPlaneClient(restClient)
	tenantManagerClient := client.NewTenantManagerClient(restClient)
	securityService := NewSecurityService("", false, "ns-1", controlPlaneClient, tenantManagerClient)
	body := map[string]interface{}{
		"tenantId": "id",
	}
	err := securityService.CheckTenantId(context.Background(), body, "id")
	suite.ErrorContains(err, "forbidden! TenantId is not found in tenants list from tenant-manager")
}

func (suite *SecServiceTestSuite) TestCheckAnyRoleMatched_EmptyRoles() {
	matched := CheckAnyRoleMatched([]string{}, []string{})
	suite.Equal(false, matched)
}

func (suite *SecServiceTestSuite) TestCheckAnyRoleMatched_DifferentRoles() {
	matched := CheckAnyRoleMatched([]string{"ROLE_1"}, []string{"ROLE_2"})
	suite.Equal(false, matched)
}

func (suite *SecServiceTestSuite) TestCheckAnyRoleMatched_SameRoles() {
	requiredRoles := []string{"ROLE_1", "2"}
	rolesFromToken := []string{"ROLE_1", "ROLE_2"}
	matched := CheckAnyRoleMatched(requiredRoles, rolesFromToken)
	suite.Equal(true, matched)
}

func serialize(obj interface{}) string {
	bytes, err := json.Marshal(obj)
	if err != nil {
		panic(err)
	}
	return string(bytes)
}

func (suite *SecServiceTestSuite) getMockRestClient(respBody string, httpCode int) *client.RestClient {
	restClient := client.NewRestClient("")
	restClient.GetToken = func(ctx context.Context) (string, error) {
		return "", nil
	}
	restClient.Do = func(request *fasthttp.Request, response *fasthttp.Response) error {
		response.SetBodyString(respBody)
		response.SetStatusCode(httpCode)
		return nil
	}
	return restClient
}
