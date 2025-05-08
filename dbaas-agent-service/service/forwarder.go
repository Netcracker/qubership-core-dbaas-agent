package service

import (
	"context"
	"encoding/base64"
	"time"

	"github.com/netcracker/qubership-core-lib-go/v3/context-propagation/ctxhelper"
	"github.com/netcracker/qubership-core-lib-go/v3/logging"
	"github.com/netcracker/qubership-core-lib-go/v3/utils"
	"github.com/valyala/fasthttp"
)

type Forwarder struct {
	dbaasBaseUrl string
	credentials  *BasicCreds
	client       *fasthttp.Client
}

func NewForwarder(dbaasBaseUrl string, credentials *BasicCreds, client *fasthttp.Client) *Forwarder {
	return &Forwarder{
		dbaasBaseUrl: dbaasBaseUrl,
		credentials:  credentials,
		client:       client,
	}
}

func DefaultHttpClient(requestTimeout time.Duration) *fasthttp.Client {
	tlsConfig := utils.GetTlsConfig()
	return &fasthttp.Client{
		TLSConfig:                     tlsConfig,
		MaxIdleConnDuration:           30 * time.Second,
		DisableHeaderNamesNormalizing: true,
		DisablePathNormalizing:        true,
		ReadTimeout:                   requestTimeout,
		WriteTimeout:                  requestTimeout,
		DialDualStack:                 true,
	}
}

type BasicCreds struct {
	Username string
	Password []byte
}

func NewBasicCreds(username string, password []byte) *BasicCreds {
	return &BasicCreds{Username: username, Password: password}
}

func (basicCreds *BasicCreds) EncodeToBase64() string {
	plane := basicCreds.Username + ":" + string(basicCreds.Password)
	return base64.StdEncoding.EncodeToString([]byte(plane))
}

var logger logging.Logger

func init() {
	logger = logging.GetLogger("forwarder")
}

func (forwarder *Forwarder) DoRequest(ctx context.Context, method, path string, body []byte) (*fasthttp.Response, error) {
	req := fasthttp.AcquireRequest()
	req.Header.SetMethod(method)
	req.SetRequestURI(forwarder.dbaasBaseUrl + path)
	req.Header.Add("Content-Type", "application/json")
	ctxhelper.AddSerializableContextData(ctx, req.Header.Add)
	req.SetBody(body)
	if forwarder.credentials != nil {
		req.Header.Add("Authorization", "Basic "+forwarder.credentials.EncodeToBase64())
	}
	logger.DebugC(ctx, "Forwarding request is: %v", req)

	resp := fasthttp.AcquireResponse()
	err := forwarder.client.Do(req, resp)

	fasthttp.ReleaseRequest(req)
	if err != nil {
		logger.ErrorC(ctx, "Failed to forward request: %v", err)
	}
	return resp, err
}
