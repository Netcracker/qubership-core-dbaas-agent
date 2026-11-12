package client

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/netcracker/qubership-core-lib-go/v3/configloader"
	constants "github.com/netcracker/qubership-core-lib-go/v3/const"
	"github.com/netcracker/qubership-core-lib-go/v3/context-propagation/ctxhelper"
	"github.com/netcracker/qubership-core-lib-go/v3/logging"
	"github.com/netcracker/qubership-core-lib-go/v3/security/rest"
	"github.com/netcracker/qubership-core-lib-go/v3/utils"
	"github.com/valyala/fasthttp"
)

type TokenSender interface {
	Send(ctx context.Context, targetUrl string, send rest.SendFunc) error
}

type RestClient struct {
	Sender     TokenSender
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
	return &RestClient{
		Sender:     rest.NewM2MRequestSender(),
		Do:         httpclient.Do,
		client:     httpclient,
		gatewayUrl: gatewayUrl,
	}
}

func (rc *RestClient) DoRequest(ctx context.Context, method string, url string, data []byte, log logging.Logger) ([]byte, int, error) {
	fullUrl := rc.fullUrl(url)
	var response *fasthttp.Response
	defer func() {
		if response != nil {
			fasthttp.ReleaseResponse(response)
		}
	}()
	err := rc.Sender.Send(ctx, fullUrl, func(token string) (int, error) {
		// Send may call this function twice (resend after 401); keep only the last response.
		if response != nil {
			fasthttp.ReleaseResponse(response)
			response = nil
		}
		req, err := rc.constructRequest(ctx, method, fullUrl, token, data, log)
		defer fasthttp.ReleaseRequest(req)
		if err != nil {
			log.ErrorC(ctx, "Fail building request object: %+v. URL: %s, method: %s", err, fullUrl, method)
			return 0, err
		}
		response = fasthttp.AcquireResponse()
		if err := rc.Do(req, response); err != nil {
			log.WarnC(ctx, "Secured %s request to %s failed with error: %s", method, fullUrl, err)
			return 0, err
		}
		return response.StatusCode(), nil
	})
	if err != nil {
		return nil, 0, err
	}
	if response == nil {
		return nil, 0, errors.New("no response received")
	}
	respBody := append([]byte(nil), response.Body()...)
	return respBody, response.StatusCode(), nil
}

func (rc *RestClient) fullUrl(url string) string {
	var delimiter = ""
	if url[0:1] != "/" {
		delimiter = "/"
	}
	return rc.gatewayUrl + delimiter + url
}

func (rc *RestClient) constructRequest(ctx context.Context, method string, url string, m2mToken string, data []byte, logger logging.Logger) (*fasthttp.Request, error) {
	req := fasthttp.AcquireRequest()
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
