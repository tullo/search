package main

import (
	"html"
	"io"
	"log"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"os"
	"os/signal"
	"regexp"
	"syscall"
	"testing"
	"time"

	"github.com/golangcollege/sessions"
)

// Capture the CSRF token value from the HTML page
var csrfTokenRX = regexp.MustCompile(`<input type='hidden' name='csrf_token' value='(.+)'>`)

func extractCSRFToken(t *testing.T, body []byte) string {
	// extract the token from the HTML body
	matches := csrfTokenRX.FindSubmatch(body)
	// expecting an array with at least two entries (matched pattern & captured data)
	if len(matches) < 2 {
		t.Log("Matched pattern:", string(matches[0]))
		t.Fatal("No csrf token found in body")
	}

	// unescape the rendered and html escaped base64 encoded string value
	return html.UnescapeString(string(matches[1]))
}

// newTestApplication creates an application struct with mock loggers
func newTestApplication(t *testing.T) *application {
	// Initialize template cache.
	templateCache, err := newTemplateCache("./../../ui/html/")
	if err != nil {
		t.Fatal(err)
	}

	// Session manager instance that mirrors production settings.
	// Sample generation of secret bytes 'openssl rand -base64 32'.
	session := sessions.New([]byte("zBtjT1J8wWrvUCuEZf+YbBa41nKYlCKiNLeS5AGdmiQ="))
	// sessions expire after 12 hours
	session.Lifetime = 12 * time.Hour
	// Set the secure flag on session cookies.
	session.Secure = true
	// Mitigate cross site request forgry (CSRF).
	session.SameSite = http.SameSiteStrictMode

	shutdown := make(chan os.Signal, 1)
	signal.Notify(shutdown, os.Interrupt, syscall.SIGTERM)

	baseURL := "http://0.0.0.0:3000/v1"

	debugURL := "http://0.0.0.0:4000/debug"

	// Identity Provider signing key ID.
	keyID := "54bb2165-71e1-41a6-af3e-7da4a0e1e2c1"

	// App struct instantiation using mocks for loggers and database models.
	app := application{
		debug:         true,
		debugURL:      debugURL,
		keyID:         keyID,
		log:           log.New(io.Discard, "", 0),
		templateCache: templateCache,
		salesURL:      baseURL,
		session:       session,
		shutdown:      shutdown,
		useTLS:        true,
	}

	return &app
}

type testServer struct {
	*httptest.Server
}

// newTestServer initalizes and returns a new instance of testServer
func newTestServer(t *testing.T, h http.Handler) *testServer {

	// spinup a https server for the duration of the test
	ts := httptest.NewUnstartedServer(h)
	ts.EnableHTTP2 = true
	ts.StartTLS()

	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatal(err)
	}

	// add the cookie jar to the client, so that response cookies are stored
	// and then sent with subsequent requests
	ts.Client().Jar = jar

	// disabling the default behaviour for redirect-following for the client
	// returning the error forces it to immediately return the received response
	ts.Client().CheckRedirect = func(req *http.Request, via []*http.Request) error {
		return http.ErrUseLastResponse
	}

	return &testServer{ts}
}

// get performs a GET request to a given url path on the test server
func (ts *testServer) get(t *testing.T, urlPath string) (int, http.Header, []byte) {
	// make a GET request against the test server
	rs, err := ts.Client().Get(ts.URL + urlPath)
	if err != nil {
		t.Fatal(err)
	}
	defer rs.Body.Close()

	body, err := io.ReadAll(rs.Body)
	if err != nil {
		t.Fatal(err)
	}

	return rs.StatusCode, rs.Header, body
}

// postForm method for sending POST requests to the test server
func (ts *testServer) postForm(t *testing.T, urlPath string, form url.Values) (int, http.Header, []byte) {
	// make a POST request against the test server
	rs, err := ts.Client().PostForm(ts.URL+urlPath, form)
	if err != nil {
		t.Fatal(err)
	}
	defer rs.Body.Close()

	body, err := io.ReadAll(rs.Body)
	if err != nil {
		t.Fatal(err)
	}

	return rs.StatusCode, rs.Header, body
}

func (ts *testServer) clientDo(t *testing.T, r *http.Request) (int, http.Header, []byte) {
	rs, err := ts.Client().Do(r)
	if err != nil {
		t.Fatal(err)
	}
	defer rs.Body.Close()

	body, err := io.ReadAll(rs.Body)
	if err != nil {
		t.Fatal(err)
	}

	return rs.StatusCode, rs.Header, body
}

/*
func (ts *testServer) getCheckRedirect(t *testing.T, urlPath string, s *sessions.Session) (int, http.Header, []byte) {
	client := ts.Client()
	client.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		log.Printf("redirect for: %s", urlPath)
		for key, val := range via[0].Header {
			req.Header[key] = val
		}
		if s != nil {
			if tkn := s.GetString(req, "jsonWebToken"); tkn != "" {
				req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", tkn))
			}
		}
		return nil
	}

	// make a GET request against the test server
	rs, err := client.Get(ts.URL + urlPath)
	if err != nil {
		t.Fatal(err)
	}
	defer rs.Body.Close()

	body, err := io.ReadAll(rs.Body)
	if err != nil {
		t.Fatal(err)
	}

	return rs.StatusCode, rs.Header, body
}
*/
