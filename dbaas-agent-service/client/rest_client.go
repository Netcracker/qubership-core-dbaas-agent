package client

import (
	"context"
	"fmt"
	"time"

	"github.com/netcracker/qubership-core-lib-go/v3/configloader"
	constants "github.com/netcracker/qubership-core-lib-go/v3/const"
	"github.com/netcracker/qubership-core-lib-go/v3/context-propagation/ctxhelper"
	"github.com/netcracker/qubership-core-lib-go/v3/logging"
	"github.com/netcracker/qubership-core-lib-go/v3/security"
	"github.com/netcracker/qubership-core-lib-go/v3/serviceloader"
	"github.com/netcracker/qubership-core-lib-go/v3/utils"
	"github.com/valyala/fasthttp"
)

type RestClient struct {
	GetToken   func(ctx context.Context) (string, error)
	Do         func(*fasthttp.Request, *fasthttp.Response) error
	client     *fasthttp.Client
	gatewayUrl string
}

func NewRestClient(gatewayUrl string) *RestClient {
	httpclient := &fasthttp.Client{
		MaxIdleConnDuration:           30 * time.Second,
		DisableHeaderNamesNormalizing: true,
		DisablePathNormalizing:        true,
		TLSConfig:                     utils.GetTlsConfig(),
		DialDualStack:                 true,
	}
	tokenProvider := serviceloader.MustLoad[security.TokenProvider]()
	return &RestClient{
		GetToken:   tokenProvider.GetToken,
		Do:         httpclient.Do,
		client:     httpclient,
		gatewayUrl: gatewayUrl,
	}
}

func (rc *RestClient) DoRequest(ctx context.Context, method string, url string, data []byte, log logging.Logger) ([]byte, int, error) {
	req, err := rc.constructRequest(ctx, method, url, data, logger)
	defer fasthttp.ReleaseRequest(req)
	if err != nil {
		log.ErrorC(ctx, "Fail building request object: %+v. URL: %s, method: %s", err, url, method)
		return nil, 0, err
	}
	response := fasthttp.AcquireResponse()
	defer fasthttp.ReleaseResponse(response)
	err = rc.Do(req, response)
	if err != nil {
		log.WarnC(ctx, "Secured %s request to %s failed with error: %s, retrying", method, url, err)
		return nil, 0, err
	}
	return response.Body(), response.StatusCode(), nil
}

func (rc *RestClient) constructRequest(ctx context.Context, method string, url string, data []byte, logger logging.Logger) (*fasthttp.Request, error) {
	req := fasthttp.AcquireRequest()
	var delimiter = ""
	if url[0:1] != "/" {
		delimiter = "/"
	}
	url = rc.gatewayUrl + delimiter + url
	m2mToken, err := rc.GetToken(ctx)
	if err != nil {
		logger.ErrorC(ctx, "Can't get M2M token %v", err)
		return req, err
	}
	req.Header.Add("Authorization", fmt.Sprintf("Bearer %s", m2mToken))
	req.Header.Add("Content-Type", "application/json")

	logger.Debugf(`Building secure request with arguments:
	method=%v, 
	url=%v`, method, url)

	req.Header.SetMethod(method)
	req.SetRequestURI(url)
	if data != nil {
		req.SetBody(data)
	}

	if err := ctxhelper.AddSerializableContextData(ctx, req.Header.Set); err != nil {
		logger.ErrorC(ctx, "Error during context serializing: %+v", err)
		return req, err
	}

	return req, nil
}

func GetInternalGatewayUrl() string {
	apigatewayUrlHttp := configloader.GetOrDefaultString("apigateway.internal.url", constants.DefaultHttpGatewayUrl)
	apigatewayUrlHttps := configloader.GetOrDefaultString("apigateway.internal.url-https", constants.DefaultHttpsGatewayUrl)
	return constants.SelectUrl(apigatewayUrlHttp, apigatewayUrlHttps)
}
