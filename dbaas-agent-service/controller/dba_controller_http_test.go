package controller

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"time"

	"github.com/gofiber/fiber/v3"
	"github.com/netcracker/qubership-core-dbaas-agent/dbaas-agent-service/v2/service"
	"github.com/netcracker/qubership-core-dbaas-agent/dbaas-agent-service/v2/testutils"
	fiberserver "github.com/netcracker/qubership-core-lib-go-fiber-server-utils/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func (suite *TestSuite) TestForwardedRequestPath() {
	upstream := &recordingUpstream{}
	testutils.AddHandler(testutils.Contains("/api/composite/v1/structures"), upstream.ServeHTTP)
	app := suite.newHttpApp()
	const forwarded = `{"forwarded":true}`
	tests := []struct {
		name            string
		target          string
		wantUpstreamURI string
	}{
		{name: "plain path", target: "/api/composite/v1/structures/abc", wantUpstreamURI: "/api/composite/v1/structures/abc"},
		{name: "dot-dot segment", target: "/api/composite/v1/structures/x/../abc", wantUpstreamURI: "/api/composite/v1/structures/abc"},
		{name: "encoded dot-dot segment", target: "/api/composite/v1/structures/x/%2e%2e/abc", wantUpstreamURI: "/api/composite/v1/structures/abc"},
		{name: "percent-encoded unreserved character", target: "/api/composite/v1/structures/%61bc", wantUpstreamURI: "/api/composite/v1/structures/abc"},
		{name: "query string", target: "/api/composite/v1/structures/abc?x=1", wantUpstreamURI: "/api/composite/v1/structures/abc"},
	}
	for _, tt := range tests {
		suite.Run(tt.name, func() {
			upstream.reset()
			req := httptest.NewRequest(http.MethodGet, tt.target, nil)
			req.Header.Set("X-Request-Id", "req-1")

			status, contentType, body := suite.send(app, req)

			received := upstream.last()
			assert.Equal(suite.T(), http.StatusOK, status, "status")
			assert.Equal(suite.T(), "application/json", contentType, "Content-Type")
			assert.Equal(suite.T(), forwarded, body, "body")
			assert.Equal(suite.T(), tt.wantUpstreamURI, received.requestURI, "request URI the upstream received")
			assert.Equal(suite.T(), "req-1", received.requestID, "X-Request-Id the upstream received")
		})
	}
}

func (suite *TestSuite) TestRequestBodyContentType() {
	app := suite.newHttpApp()
	const parsed = `{"error":"request must contain a classifier"}`
	const notParsed = `{"error":"Failed to read request body in create database handler"}`
	tests := []struct {
		name        string
		contentType string
		body        string
		wantStatus  int
		wantBody    string
	}{
		{name: "application/json", contentType: "application/json", body: `{"type":"postgresql"}`, wantStatus: http.StatusBadRequest, wantBody: parsed},
		{name: "application/json with charset", contentType: "application/json; charset=utf-8", body: `{"type":"postgresql"}`, wantStatus: http.StatusBadRequest, wantBody: parsed},
		{name: "text/json", contentType: "text/json", body: `{"type":"postgresql"}`, wantStatus: http.StatusInternalServerError, wantBody: notParsed},
		{name: "vendor +json type", contentType: "application/vnd.netcracker+json", body: `{"type":"postgresql"}`, wantStatus: http.StatusBadRequest, wantBody: parsed},
		{name: "no Content-Type", body: `{"type":"postgresql"}`, wantStatus: http.StatusInternalServerError, wantBody: notParsed},
		{name: "text/plain", contentType: "text/plain", body: `{"type":"postgresql"}`, wantStatus: http.StatusInternalServerError, wantBody: notParsed},
		{name: "malformed JSON", contentType: "application/json", body: `{"type":`, wantStatus: http.StatusInternalServerError, wantBody: notParsed},
		{name: "empty body", contentType: "application/json", wantStatus: http.StatusBadRequest, wantBody: `{"error":"request body in create database handler is nil"}`},
	}
	for _, tt := range tests {
		suite.Run(tt.name, func() {
			req := httptest.NewRequest(http.MethodPut, "/api/v3/dbaas/"+suite.namespace+"/databases", strings.NewReader(tt.body))
			if tt.contentType != "" {
				req.Header.Set("Content-Type", tt.contentType)
			}

			status, contentType, body := suite.send(app, req)

			assert.Equal(suite.T(), tt.wantStatus, status, "status")
			assert.Equal(suite.T(), "application/json", contentType, "Content-Type")
			assert.Equal(suite.T(), tt.wantBody, body, "body")
		})
	}
}

// newHttpApp registers the handlers on the route patterns the service uses, with a forwarder that sends requests
// through the service's own HTTP client: unlike the suite's client, it does not normalize the path it sends.
func (suite *TestSuite) newHttpApp() *fiber.App {
	app, err := fiberserver.New().Process()
	require.NoError(suite.T(), err)
	forwarder := service.NewForwarder(testutils.GetMockServerUrl(), service.NewBasicCreds("test-user", []byte("test-pass")), service.DefaultHttpClient(5*time.Second))
	controller := NewController(service.NewSecurityService("", false, suite.namespace, nil, nil), forwarder)
	app.Put("/api/v3/dbaas/:namespace/databases", controller.HandleGetOrCreateDatabaseV3)
	app.Get("/api/composite/v1/structures/:compositeId", controller.ForwardHandler)
	return app
}

func (suite *TestSuite) send(app *fiber.App, req *http.Request) (int, string, string) {
	resp, err := app.Test(req, fiber.TestConfig{Timeout: 0})
	require.NoError(suite.T(), err, "%s %s", req.Method, req.RequestURI)
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	require.NoError(suite.T(), err, "read response body")
	return resp.StatusCode, resp.Header.Get("Content-Type"), string(body)
}

type upstreamRequest struct {
	requestURI string
	requestID  string
}

type recordingUpstream struct {
	mu      sync.Mutex
	request upstreamRequest
}

func (u *recordingUpstream) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	u.mu.Lock()
	u.request = upstreamRequest{requestURI: r.RequestURI, requestID: r.Header.Get("X-Request-Id")}
	u.mu.Unlock()
	w.Header().Set("Content-Type", "application/json")
	_, _ = io.WriteString(w, `{"forwarded":true}`)
}

func (u *recordingUpstream) reset() {
	u.mu.Lock()
	defer u.mu.Unlock()
	u.request = upstreamRequest{}
}

func (u *recordingUpstream) last() upstreamRequest {
	u.mu.Lock()
	defer u.mu.Unlock()
	return u.request
}
