package main

import (
	"context"
	"fmt"
	"log"
	"net/http"

	"github.com/justinas/nosurf"
)

func secureHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-XSS-Protection", "1; mode=block")
		w.Header().Set("X-Frame-Options", "deny")

		next.ServeHTTP(w, r)
	})
}

// noSurf uses a customized CSRF cookie with the Secure, Path and HttpOnly flags set
func noSurf(next http.Handler) http.Handler {
	csrfHandler := nosurf.New(next)
	paths := []string{"/ping", "/about"}
	csrfHandler.ExemptPaths(paths...)
	csrfHandler.SetBaseCookie(http.Cookie{
		Domain:   "",
		HttpOnly: true,
		MaxAge:   24 * 60 * 60, // 24 hours
		Path:     "/",
		SameSite: http.SameSiteStrictMode,
		Secure:   true, // for transport over https
	})
	csrfHandler.SetFailureHandler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		log.Println("CSRF failure:", nosurf.Reason(r))
	}))

	return csrfHandler
}

func (app *application) logRequest(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		app.log.Printf("%s - %s %s %s", r.RemoteAddr, r.Proto, r.Method, r.URL.RequestURI())

		next.ServeHTTP(w, r)
	})
}

// recoverPanic recovers the panic and logs the cause
func (app *application) recoverPanic(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// called last on the way up in the middleware chain while Go unwinds the stack
		defer func() {
			// check if there has been a panic or not
			if err := recover(); err != nil {
				// trigger the Go server to automatically close the current connection
				// after a response has been sent.
				w.Header().Set("Connection", "close")
				// format error with default textual representation
				app.serverError(w, fmt.Errorf("%s", err))
			}
		}()

		next.ServeHTTP(w, r)
	})
}

// requireAuthentication redirects the unauthenticated user to the login page
func (app *application) requireAuthentication(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !app.isAuthenticated(r) {
			// add the path the user is trying to access to session data
			app.session.Put(r, "redirectPathAfterLogin", r.URL.Path)
			http.Redirect(w, r, "/user/login", http.StatusSeeOther)
			return
		}
		// pages that require authentication should not be stored in caches
		// (browser cache or other intermediary cache)
		w.Header().Add("Cache-Control", "no-store")

		next.ServeHTTP(w, r)
	})
}

// authenticate checks the database for user status (active)
func (app *application) authenticate(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// check if user is logged in
		exists := app.session.Exists(r, "authenticatedUserID")
		if !exists {
			next.ServeHTTP(w, r)
			return
		}

		// request is coming from an authenticated & 'active' user,
		// add key/value pair to the request context - to be used further down the chain
		ctx := context.WithValue(r.Context(), contextKeyIsAuthenticated, true)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}
