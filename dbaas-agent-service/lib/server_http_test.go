package lib

import (
	"bufio"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/netcracker/qubership-core-dbaas-agent/dbaas-agent-service/v2/controller"
	"github.com/netcracker/qubership-core-dbaas-agent/dbaas-agent-service/v2/service"
	"github.com/netcracker/qubership-core-lib-go-actuator-common/v2/apiversion"
	"github.com/netcracker/qubership-core-lib-go-fiber-server-utils/v2/security"
	"github.com/netcracker/qubership-core-lib-go/v3/configloader"
	"github.com/netcracker/qubership-core-lib-go/v3/serviceloader"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const testNamespace = "test-namespace"

func TestForwardedRequest(t *testing.T) {
	srv := startServer(t)
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
		t.Run(tt.name, func(t *testing.T) {
			resp := sendRaw(t, srv.port, "GET "+tt.target+" HTTP/1.1\r\nHost: localhost\r\nX-Request-Id: req-1\r\nConnection: close\r\n\r\n")

			upstream := srv.upstream.last()
			assert.Equal(t, http.StatusOK, resp.status, "status, body %q", resp.body)
			assert.Equal(t, "application/json", resp.contentType, "Content-Type")
			assert.Equal(t, `{"forwarded":true}`, resp.body, "body")
			assert.Equal(t, tt.wantUpstreamURI, upstream.requestURI, "request URI the upstream received")
			assert.Equal(t, "req-1", upstream.requestID, "X-Request-Id the upstream received")
		})
	}
}

func TestRequestBodyParsing(t *testing.T) {
	srv := startServer(t)
	const parsed = `{"error":"Got error during work with tenant context"}`
	const notParsed = `{"error":"Failed to read request body in create database handler"}`
	tests := []struct {
		name        string
		contentType string
		body        string
		wantStatus  int
		wantBody    string
	}{
		{name: "application/json", contentType: "application/json", body: `{"type":"postgresql"}`, wantStatus: http.StatusForbidden, wantBody: parsed},
		{name: "application/json with charset", contentType: "application/json; charset=utf-8", body: `{"type":"postgresql"}`, wantStatus: http.StatusForbidden, wantBody: parsed},
		{name: "text/json", contentType: "text/json", body: `{"type":"postgresql"}`, wantStatus: http.StatusForbidden, wantBody: parsed},
		{name: "vendor +json type", contentType: "application/vnd.netcracker+json", body: `{"type":"postgresql"}`, wantStatus: http.StatusForbidden, wantBody: parsed},
		{name: "no Content-Type", body: `{"type":"postgresql"}`, wantStatus: http.StatusInternalServerError, wantBody: notParsed},
		{name: "text/plain", contentType: "text/plain", body: `{"type":"postgresql"}`, wantStatus: http.StatusInternalServerError, wantBody: notParsed},
		{name: "malformed JSON", contentType: "application/json", body: `{"type":`, wantStatus: http.StatusInternalServerError, wantBody: notParsed},
		{name: "empty body", contentType: "application/json", wantStatus: http.StatusBadRequest, wantBody: `{"error":"request body in create database handler is nil"}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			request := "PUT /api/v3/dbaas/" + testNamespace + "/databases HTTP/1.1\r\nHost: localhost\r\nConnection: close\r\n"
			if tt.contentType != "" {
				request += "Content-Type: " + tt.contentType + "\r\n"
			}
			request += "Content-Length: " + strconv.Itoa(len(tt.body)) + "\r\n\r\n" + tt.body

			resp := sendRaw(t, srv.port, request)

			assert.Equal(t, tt.wantStatus, resp.status, "status")
			assert.Equal(t, "application/json", resp.contentType, "Content-Type")
			assert.Equal(t, tt.wantBody, resp.body, "body")
		})
	}
}

func TestRoutingResponses(t *testing.T) {
	srv := startServer(t)
	tests := []struct {
		name            string
		path            string
		wantStatus      int
		wantContentType string
		wantBody        string
	}{
		{name: "liveness probe", path: "/probes/live", wantStatus: http.StatusOK, wantContentType: "text/plain; charset=utf-8", wantBody: ""},
		{name: "unknown path", path: "/missing", wantStatus: http.StatusNotFound, wantContentType: "text/plain; charset=utf-8", wantBody: "Cannot GET /missing"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resp := sendRaw(t, srv.port, "GET "+tt.path+" HTTP/1.1\r\nHost: localhost\r\nConnection: close\r\n\r\n")

			assert.Equal(t, tt.wantStatus, resp.status, "status")
			assert.Equal(t, tt.wantContentType, resp.contentType, "Content-Type")
			assert.Equal(t, tt.wantBody, resp.body, "body")
		})
	}
}

func TestServerAcceptsIPv4AndIPv6(t *testing.T) {
	srv := startServer(t)
	for _, host := range []string{"127.0.0.1", "::1"} {
		t.Run(host, func(t *testing.T) {
			if host == "::1" && !ipv6LoopbackAvailable() {
				t.Skip("IPv6 loopback is not available on this machine")
			}
			url := fmt.Sprintf("http://%s/probes/live", net.JoinHostPort(host, strconv.Itoa(srv.port)))
			client := &http.Client{Timeout: 2 * time.Second}

			resp, err := client.Get(url)

			require.NoError(t, err, "GET %s", url)
			_ = resp.Body.Close()
			assert.Equal(t, http.StatusOK, resp.StatusCode, "status of GET %s", url)
		})
	}
}

type testServer struct {
	port     int
	upstream *recordingUpstream
	err      error
}

var (
	sharedServer     testServer
	sharedServerOnce sync.Once
)

// startServer starts the application once per test binary: WithPprof binds the fixed port 6060, so a second
// instance in the same process would fail to start its pprof listener.
func startServer(t *testing.T) testServer {
	t.Helper()
	sharedServerOnce.Do(func() {
		sharedServer = launchServer()
	})
	require.NoError(t, sharedServer.err, "start the application")
	return sharedServer
}

func launchServer() testServer {
	upstream := &recordingUpstream{}
	upstreamServer := httptest.NewServer(upstream)
	port, err := freePort()
	if err != nil {
		return testServer{err: err}
	}
	for key, value := range map[string]string{
		"HTTP_SERVER_BIND":       ":" + strconv.Itoa(port),
		"MICROSERVICE_NAME":      "dbaas-agent",
		"MICROSERVICE_NAMESPACE": testNamespace,
	} {
		if err := os.Setenv(key, value); err != nil {
			return testServer{err: err}
		}
	}
	configloader.Init(configloader.EnvPropertySource())
	serviceloader.Register(1, &security.DummyFiberServerSecurityMiddleware{})

	forwarder = service.NewForwarder(upstreamServer.URL, service.NewBasicCreds("user", []byte("password")), service.DefaultHttpClient(5*time.Second))
	securityService = service.NewSecurityService("", false, testNamespace, nil, nil)
	apiController = controller.NewController(securityService, forwarder)
	if apiVersionService, err = service.NewApiVersionService(apiversion.ApiVersionConfig{}, forwarder); err != nil {
		return testServer{err: err}
	}
	app, err := newApp()
	if err != nil {
		return testServer{err: err}
	}
	go listen(app)

	deadline := time.Now().Add(10 * time.Second)
	for {
		conn, err := net.DialTimeout("tcp", "127.0.0.1:"+strconv.Itoa(port), time.Second)
		if err == nil {
			_ = conn.Close()
			return testServer{port: port, upstream: upstream}
		}
		if time.Now().After(deadline) {
			return testServer{err: fmt.Errorf("application did not start listening on port %d: %w", port, err)}
		}
		time.Sleep(50 * time.Millisecond)
	}
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

func (u *recordingUpstream) last() upstreamRequest {
	u.mu.Lock()
	defer u.mu.Unlock()
	return u.request
}

type rawResult struct {
	status      int
	contentType string
	body        string
}

func sendRaw(t *testing.T, port int, request string) rawResult {
	t.Helper()
	conn, err := net.DialTimeout("tcp", "127.0.0.1:"+strconv.Itoa(port), 2*time.Second)
	require.NoError(t, err, "dial the application")
	defer conn.Close()
	require.NoError(t, conn.SetDeadline(time.Now().Add(5*time.Second)))
	_, err = io.WriteString(conn, request)
	require.NoError(t, err, "write request %q", request)
	resp, err := http.ReadResponse(bufio.NewReader(conn), nil)
	require.NoError(t, err, "read response to %q", strings.SplitN(request, "\r\n", 2)[0])
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err, "read response body")
	return rawResult{status: resp.StatusCode, contentType: resp.Header.Get("Content-Type"), body: string(body)}
}

func freePort() (int, error) {
	ln, err := net.Listen("tcp", ":0")
	if err != nil {
		return 0, err
	}
	defer ln.Close()
	return ln.Addr().(*net.TCPAddr).Port, nil
}

func ipv6LoopbackAvailable() bool {
	ln, err := net.Listen("tcp6", "[::1]:0")
	if err != nil {
		return false
	}
	_ = ln.Close()
	return true
}
