package testutils

import (
	"github.com/valyala/fasthttp"
	"net/http"
	"net/http/httptest"
	"strings"
)

var (
	mockHandlers []mockHandler
	mockServer   *httptest.Server
)

type mockHandler struct {
	pathMatcher func(path string) bool
	handle      func(w http.ResponseWriter, r *http.Request)
}

func StartMockServer() {
	mockServer = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestPath := r.URL.Path
		for _, handler := range mockHandlers {
			if handler.pathMatcher(requestPath) {
				handler.handle(w, r)
			}
		}
	}))
	// need to replace address or testcontainers won't start
	url := strings.Replace(mockServer.URL, "127.0.0.1", "localhost", 1)
	mockServer.URL = url
}

func StopMockServer() {
	mockServer.Close()
	ClearHandlers()
}

func AddHandler(pathMatcher func(string) bool, handler func(http.ResponseWriter, *http.Request)) {
	mockHandlers = append(mockHandlers, mockHandler{
		pathMatcher: pathMatcher,
		handle:      handler,
	})
}

func ClearHandlers() {
	mockHandlers = nil
}

func Contains(submatch string) func(string) bool {
	return func(path string) bool {
		return strings.Contains(path, submatch)
	}
}

func GetMockServerUrl() string {
	return mockServer.URL
}

func GetMockServerClient() *fasthttp.Client {
	return &fasthttp.Client{}
}
