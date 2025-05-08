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

const getTenantsUrl = "/api/v4/tenant-manager/manage/tenants"

var loggerTM logging.Logger

func init() {
	loggerTM = logging.GetLogger("tenant-manager-client")
}

type TenantManagerClient struct {
	restClient      *RestClient
	tenantListCache []Tenant
}

type Tenant struct {
	TenantId string `json:"externalId"`
}

func NewTenantManagerClient(restClient *RestClient) *TenantManagerClient {
	return &TenantManagerClient{restClient: restClient}
}

func (tmClient *TenantManagerClient) GetTenantsList(ctx context.Context) ([]Tenant, error) {
	loggerTM.DebugC(ctx, "request to get tenants list from tenant-manager")
	resp, httpCode, err := tmClient.restClient.DoRequest(ctx, fasthttp.MethodGet, getTenantsUrl, nil, loggerTM)
	loggerTM.InfoC(ctx, "tenants list response httpCode %d, error: %+v", httpCode, err)
	if err != nil {
		return nil, errors.New(fmt.Sprintf("error getting tenants list from tenant-manager: %s", err))
	}
	if httpCode != http.StatusOK {
		return nil, errors.New(fmt.Sprintf("tenant-manager returned unexpected code. Expected 200 but got %d, response: %s", httpCode, resp))
	}
	var tenantsList []Tenant

	if err := json.Unmarshal(resp, &tenantsList); err != nil {
		return nil, errors.New("failed to parse tenants response body" + err.Error())
	}
	tmClient.tenantListCache = tenantsList
	return tenantsList, nil
}

func (tmClient *TenantManagerClient) GetTenantsFromCache() []Tenant {
	return tmClient.tenantListCache
}

func (tmClient *TenantManagerClient) CleanCacheJob() {
	tmClient.tenantListCache = nil
}
