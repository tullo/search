package main

import (
	"bytes"
	"context"
	"fmt"
	"log"
	"net/http"
	"runtime/debug"
	"time"

	"github.com/justinas/nosurf"
)

func newClient() *http.Client {
	var client http.Client
	t := http.DefaultTransport.(*http.Transport)
	client.Transport = t.Clone()
	return &client
}

func (app *application) newGetRequest(ctx context.Context, r *http.Request, url string) (*http.Request, error) {
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}

	// Bind the new context into the request.
	req = req.WithContext(ctx)

	if tkn := app.session.GetString(r, "jsonWebToken"); tkn != "" {
		req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", tkn))
	}

	return req, nil
}

func (app *application) serverError(w http.ResponseWriter, err error) {
	trace := fmt.Sprintf("%s\n%s", err.Error(), debug.Stack())
	// go one step back in the stack trace to get the file name and line number
	app.log.Output(2, trace)

	// when running in debug mode,
	// write detailed errors and stack traces to the http response
	if app.debug {
		http.Error(w, trace, http.StatusInternalServerError)
		return
	}

	http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)

	app.SignalShutdown()
}

func (app *application) clientError(w http.ResponseWriter, status int) {
	http.Error(w, http.StatusText(status), status)
}

func (app *application) addDefaultData(td *templateData, r *http.Request) *templateData {
	if td == nil {
		td = &templateData{}
	}
	td.CurrentYear = time.Now().Year()
	td.Version = build

	// add CSRF token to the template data
	td.CSRFToken = nosurf.Token(r)

	// retrieve the value for the flash key and delete the key in one step
	// add flash message to the template data
	td.Flash = app.session.PopString(r, "flash")

	// add authentication status to the template data
	td.IsAuthenticated = app.isAuthenticated(r)

	return td
}

// isAuthenticated checks if the request is from an authenticated user
func (app *application) isAuthenticated(r *http.Request) bool {
	isAuthenticated, ok := r.Context().Value(contextKeyIsAuthenticated).(bool)
	if !ok {
		// key not found in ctx, or value was not a boolean
		return false
	}
	return isAuthenticated
}

func (app *application) render(w http.ResponseWriter, r *http.Request, name string, data *templateData) {
	ts, ok := app.templateCache[name]
	if !ok {
		app.serverError(w, fmt.Errorf("the template %s does not exist", name))
		return
	}

	// stage 1: write template into buffer
	buf := new(bytes.Buffer)
	err := ts.Execute(buf, app.addDefaultData(data, r))
	if err != nil {
		log.Printf("render err=%v+\n", err)
		app.serverError(w, err)
		return
	}

	// stage 2: write rendered content
	buf.WriteTo(w)
}
