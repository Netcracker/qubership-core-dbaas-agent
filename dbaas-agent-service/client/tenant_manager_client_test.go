package client

import (
	"context"
	"errors"
	"github.com/stretchr/testify/suite"
	"net/http"
	"testing"
)

type TenantManagerClientTestSuite struct {
	suite.Suite
}

func TestTenantManagerClientTestSuite(t *testing.T) {
	suite.Run(t, new(TenantManagerClientTestSuite))
}

func (suite *TenantManagerClientTestSuite) TestGetTenantsList_TenantManagerReturnError() {
	restClient := getMockRestClient("", http.StatusInternalServerError, errors.New("unknown error"))
	tenantManagerClient := NewTenantManagerClient(restClient)
	_, err := tenantManagerClient.GetTenantsList(context.Background())
	suite.ErrorContains(err, "error getting tenants list from tenant-manager: unknown error")
}

func (suite *TenantManagerClientTestSuite) TestGetTenantsList_TenantManagerReturnUnexpectedStatus() {
	restClient := getMockRestClient("unknown error", http.StatusInternalServerError, nil)
	tenantManagerClient := NewTenantManagerClient(restClient)
	_, err := tenantManagerClient.GetTenantsList(context.Background())
	suite.ErrorContains(err, "tenant-manager returned unexpected code. Expected 200 but got 500, response: unknown error")
}

func (suite *TenantManagerClientTestSuite) TestGetTenantsList_TenantManagerNotParseableResponse() {
	restClient := getMockRestClient("bad_response", http.StatusOK, nil)
	tenantManagerClient := NewTenantManagerClient(restClient)
	_, err := tenantManagerClient.GetTenantsList(context.Background())
	suite.ErrorContains(err, "failed to parse tenants response body")
}

func (suite *TenantManagerClientTestSuite) TestGetTenantsList_GotTenantsList() {
	restClient := getMockRestClient(serialize([]Tenant{{TenantId: "test-tenant-id"}}), http.StatusOK, nil)
	tenantManagerClient := NewTenantManagerClient(restClient)
	tenantList, err := tenantManagerClient.GetTenantsList(context.Background())
	suite.Nil(err)
	suite.Equal(1, len(tenantList))
	suite.Equal("test-tenant-id", tenantList[0].TenantId)
	suite.Equal(tenantList, tenantManagerClient.GetTenantsFromCache())
}

func (suite *TenantManagerClientTestSuite) TestCleanCacheJob() {
	restClient := getMockRestClient(serialize([]Tenant{{TenantId: "test-tenant-id"}}), http.StatusOK, nil)
	tenantManagerClient := NewTenantManagerClient(restClient)
	tenantList, _ := tenantManagerClient.GetTenantsList(context.Background())
	suite.Equal(tenantList, tenantManagerClient.GetTenantsFromCache())

	tenantManagerClient.CleanCacheJob()
	suite.Nil(tenantManagerClient.GetTenantsFromCache())
}
