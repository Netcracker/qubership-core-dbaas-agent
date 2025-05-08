package service

import (
	"context"
	"encoding/base64"
	"io/ioutil"
	"math/rand"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/netcracker/qubership-core-lib-go/v3/context-propagation/baseproviders"
	"github.com/netcracker/qubership-core-lib-go/v3/context-propagation/baseproviders/xrequestid"
	"github.com/netcracker/qubership-core-lib-go/v3/context-propagation/ctxmanager"
	"github.com/valyala/fasthttp"
)

const charset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"

var seededRand = rand.New(rand.NewSource(time.Now().UnixNano()))

func GenerateRandomString(length int) string {
	b := make([]byte, length)
	for i := range b {
		b[i] = charset[seededRand.Intn(len(charset))]
	}
	return string(b)
}

func TestBasicCreds_EncodeToBase64(t *testing.T) {
	username := GenerateRandomString(8)
	password := GenerateRandomString(9)
	expected := base64.StdEncoding.EncodeToString([]byte(username + ":" + password))

	basicCreds := NewBasicCreds(username, []byte(password))

	if actual := basicCreds.EncodeToBase64(); actual != expected {
		t.Errorf("TestBasicCreds_EncodeToBase64 failed! Expected encoded base64 credentials: %v, actual: %v", expected, actual)
	}
	// one more time to verify that result can be reproduced
	if actual := basicCreds.EncodeToBase64(); actual != expected {
		t.Errorf("TestBasicCreds_EncodeToBase64 failed! Expected encoded base64 credentials: %v, actual: %v", expected, actual)
	}
}

func TestForwarder_DoRequest(t *testing.T) {
	const testPath = "/api/v1/dbaas/test-path"
	const testReqBody = `{"classifier": {"isServiceDb": true, "microserviceName": "dbaas-agent", "namespace": "dbaas-agent-it-test-namespace"}, "type": "mongodb"}`
	const testRespBody = `Fine, take your database`
	const testReqId = "12345"
	basicCreds := NewBasicCreds("testUser", []byte("pas$w0rd"))
	// Start a local HTTP server
	server := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
		if req.Method != http.MethodPut {
			t.Errorf("Wrong HTTP method in TestForwarder_DoRequest! Expected: %v, actual: %v", http.MethodPut, req.Method)
		}
		if req.URL.Path != testPath {
			t.Errorf("Wrong HTTP path in TestForwarder_DoRequest! Expected: %v, actual: %v", testPath, req.URL.Path)
		}
		if req.Header.Get("Content-Type") != "application/json" {
			t.Errorf("Missing Content-Type HTTP header in TestForwarder_DoRequest! Expected: %v, actual: %v", "application/json", req.Header.Get("Content-Type"))
		}
		if req.Header.Get(xrequestid.X_REQUEST_ID_HEADER_NAME) != testReqId {
			t.Errorf("Missing X-Request-ID HTTP header in TestForwarder_DoRequest! Expected: %v, actual: %v", testReqId, req.Header.Get(xrequestid.X_REQUEST_ID_HEADER_NAME))
		}
		if user, pass, ok := req.BasicAuth(); !ok || user != "testUser" || pass != "pas$w0rd" {
			t.Errorf("Unexpected basic authorization in TestForwarder_DoRequest! Username: %v, Password: %v, valid: %v", user, pass, ok)
		}
		reqBody, err := ioutil.ReadAll(req.Body)
		if err != nil {
			t.Errorf("Error in reading request body in TestForwarder_DoRequest! %v", err)
		}
		if string(reqBody) != testReqBody {
			t.Errorf("Wrong HTTP request body in TestForwarder_DoRequest!\r\n Expected: %v,\r\n actual: %v", testReqBody, string(reqBody))
		}

		rw.WriteHeader(http.StatusCreated)
		if _, err := rw.Write([]byte(testRespBody)); err != nil {
			t.Errorf("Error in sending response body in TestForwarder_DoRequest: %v", err)
		}
	}))
	defer server.Close()

	ctxmanager.Register(baseproviders.Get())
	forwarder := NewForwarder(server.URL, basicCreds, DefaultHttpClient(0))
	forwarder.client = &fasthttp.Client{}

	ctx := context.Background()
	reqIdProvider, _ := ctxmanager.GetProvider(xrequestid.X_REQUEST_ID_COTEXT_NAME)
	ctx = reqIdProvider.Provide(ctx, map[string]interface{}{xrequestid.X_REQUEST_ID_HEADER_NAME: testReqId})

	resp, err := forwarder.DoRequest(ctx, http.MethodPut, testPath, []byte(testReqBody))
	if err != nil {
		t.Errorf("Error in forwarder test: %v", err)
	}
	if resp.StatusCode() != fasthttp.StatusCreated {
		t.Errorf("Wrong HTTP response status code in TestForwarder_DoRequest!\r\n Expected: %v,\r\n actual: %v", http.StatusCreated, resp.StatusCode())
	}
	if string(resp.Body()) != testRespBody {
		t.Errorf("Wrong HTTP response body in TestForwarder_DoRequest!\r\n Expected: %v,\r\n actual: %v", testRespBody, string(resp.Body()))
	}
}
