package main

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"net/url"
	"testing"
)

func TestPing(t *testing.T) {
	if testing.Short() {
		t.Log("skipping")
		return
	}

	app := newTestApplication(t)
	ts := newTestServer(t, app.routes())
	defer ts.Close()

	code, _, body := ts.get(t, "/ping")
	if code != http.StatusOK {
		t.Errorf("want %d; got %d", http.StatusOK, code)
	}

	if string(body) != "OK" {
		t.Errorf("want body to equal %q", "OK")
	}
}

func TestLivenessProbe(t *testing.T) {
	if testing.Short() {
		t.Log("skipping")
		return
	}

	app := newTestApplication(t)
	ts := newTestServer(t, app.routes())
	defer ts.Close()

	url := fmt.Sprintf("%s/ping", ts.URL)
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, url, nil)
	if err != nil {
		t.Errorf("creating request %s", err)
	}

	code, _, body := ts.clientDo(t, req)

	if code != http.StatusOK {
		t.Errorf("want %d; got %d", http.StatusOK, code)
	}

	if string(body) != "OK" {
		t.Errorf("want body to equal %q", "OK")
	}
}

func TestAbout(t *testing.T) {

	app := newTestApplication(t)
	ts := newTestServer(t, app.routes())
	defer ts.Close()

	code, _, body := ts.get(t, "/about")
	if code != http.StatusOK {
		t.Errorf("want %d; got %d", http.StatusOK, code)
	}

	txt := []byte("Lorem ipsum dolor sit amet, consectetur adipiscing elit.")
	if !bytes.Contains(body, txt) {
		t.Errorf("want body to contain %q", string(txt))
	}
}

func TestLoginUser(t *testing.T) {
	if testing.Short() {
		t.Log("skipping")
		return
	}
	app := newTestApplication(t)

	ts := newTestServer(t, app.routes())
	defer ts.Close()

	_, _, body := ts.get(t, "/user/login")
	csrfToken := extractCSRFToken(t, body)

	tests := []struct {
		name         string
		userEmail    string
		userPassword string
		csrfToken    string
		wantCode     int
		wantBody     []byte
	}{
		{"Valid Submission", "user@example.com", "gophers", csrfToken, http.StatusSeeOther, nil},
		{"Empty Email", "", "validPa$$word", csrfToken, http.StatusSeeOther, []byte("Email or Password is incorrect")},
		{"Empty Password", "user@example.com", "", csrfToken, http.StatusSeeOther, []byte("Email or Password is incorrect")},
		{"Invalid Password", "user@example.com", "FooBarBaz", csrfToken, http.StatusSeeOther, []byte("Email or Password is incorrect")},
		{"Invalid CSRF Token", "", "", "wrongToken", http.StatusBadRequest, nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			form := url.Values{}
			form.Add("email", tt.userEmail)
			form.Add("password", tt.userPassword)
			form.Add("csrf_token", tt.csrfToken)

			code, _, body := ts.postForm(t, "/user/login", form)

			if code != tt.wantCode {
				t.Errorf("want %d; got %d", tt.wantCode, code)
			}

			if !bytes.Contains(body, tt.wantBody) {
				t.Errorf("want body %s to contain %q", body, tt.wantBody)
			}
		})
	}
}

func TestUserProfile(t *testing.T) {
	if testing.Short() {
		t.Log("skipping")
		return
	}

	app := newTestApplication(t)

	ts := newTestServer(t, app.routes())
	defer ts.Close()

	code, _, _ := ts.get(t, "/")
	if code != http.StatusSeeOther {
		t.Errorf("want %d; got %d", http.StatusSeeOther, code)
	}

	_, _, body := ts.get(t, "/user/login")
	csrfToken := extractCSRFToken(t, body)

	form := url.Values{}
	form.Add("email", "user@example.com")
	form.Add("password", "gophers")
	form.Add("csrf_token", csrfToken)

	// init user session by login
	_, _, _ = ts.postForm(t, "/user/login", form)

	tests := []struct {
		name     string
		urlPath  string
		wantCode int
		wantBody []byte
	}{
		{"Valid Request", "/user/profile", http.StatusOK, []byte("User Gopher")},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {

			code, _, body := ts.get(t, tt.urlPath)

			if code != tt.wantCode {
				t.Errorf("want %d; got %d", tt.wantCode, code)
			}

			if !bytes.Contains(body, tt.wantBody) {
				t.Errorf("want body to contain %q", tt.wantBody)
			}
		})
	}
}

func TestHomePage(t *testing.T) {
	if testing.Short() {
		t.Log("skipping")
		return
	}

	app := newTestApplication(t)

	ts := newTestServer(t, app.routes())
	defer ts.Close()

	code, _, _ := ts.get(t, "/")
	if code != http.StatusSeeOther {
		t.Errorf("want %d; got %d", http.StatusSeeOther, code)
	}

	_, _, body := ts.get(t, "/user/login")
	csrfToken := extractCSRFToken(t, body)

	form := url.Values{}
	form.Add("email", "user@example.com")
	form.Add("password", "gophers")
	form.Add("csrf_token", csrfToken)

	// init user session by login
	_, _, _ = ts.postForm(t, "/user/login", form)

	tests := []struct {
		name     string
		urlPath  string
		wantCode int
		wantBody []byte
	}{
		{"Valid", "/", http.StatusOK, []byte("<a href=\"/product/72f8b983-3eb4-48db-9ed0-e45cc6bd716b\">McDonalds Toys</a>")},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {

			code, _, body := ts.get(t, tt.urlPath)

			if code != tt.wantCode {
				t.Errorf("want %d; got %d", tt.wantCode, code)
			}

			if !bytes.Contains(body, tt.wantBody) {
				t.Errorf("want body to contain %q", tt.wantBody)
			}
		})
	}

}

func TestShowProduct(t *testing.T) {
	if testing.Short() {
		t.Log("skipping")
		return
	}
	app := newTestApplication(t)

	ts := newTestServer(t, app.routes())
	defer ts.Close()

	_, _, body := ts.get(t, "/user/login")
	csrfToken := extractCSRFToken(t, body)

	form := url.Values{}
	form.Add("email", "user@example.com")
	form.Add("password", "gophers")
	form.Add("csrf_token", csrfToken)

	// init user session by login
	_, _, _ = ts.postForm(t, "/user/login", form)

	tests := []struct {
		name     string
		urlPath  string
		wantCode int
		wantBody []byte
	}{
		{"Valid ID", "/product/72f8b983-3eb4-48db-9ed0-e45cc6bd716b", http.StatusOK, []byte("<h2>Product: McDonalds Toys</h2>")},
		{"ID is not in its proper form", "/product/72f8b983-fooo-baar-baaz-e45cc6bd716b", http.StatusBadRequest, nil},
		{"Non-existent ID", "/product/99f8b983-3eb4-48db-9ed0-e45cc6bd716b", http.StatusNotFound, nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {

			code, _, body := ts.get(t, tt.urlPath)

			if code != tt.wantCode {
				t.Errorf("want %d; got %d", tt.wantCode, code)
			}

			if !bytes.Contains(body, tt.wantBody) {
				t.Errorf("want body to contain %q", tt.wantBody)
			}
		})
	}
}
