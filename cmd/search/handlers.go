package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/dgrijalva/jwt-go/v4"
	"github.com/tullo/search/internal/forms"
	"github.com/tullo/search/internal/product"
	"github.com/tullo/search/internal/user"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
)

const name = "search"

func (app *application) ping(w http.ResponseWriter, r *http.Request) {

	ctx, cancel := context.WithTimeout(r.Context(), time.Second)
	defer cancel()

	url := fmt.Sprintf("%s/liveness", app.debugURL)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		w.Write([]byte(fmt.Sprintf("%v", err)))
		return
	}

	// custom header used to get around okteto related issue
	req.Header.Set("X-Probe", "LivenessProbe")

	// Client.Do will handle the context level timeout.
	client := newClient()
	resp, err := client.Do(req)
	if err != nil {
		w.Write([]byte(fmt.Sprintf("%v", err)))
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		msg := fmt.Sprintf("received unexpected response status: %d", resp.StatusCode)
		w.Write([]byte(msg))
		return
	}

	w.Write([]byte("OK"))
}

func (app *application) home(w http.ResponseWriter, r *http.Request) {

	ctx, span := otel.Tracer(name).Start(r.Context(), "home")
	defer span.End()

	// Create a context with a timeout of 1 second.
	ctx, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()

	var (
		page        = 1
		rowsPerPage = 20
	)
	url := fmt.Sprintf("%s/products/%d/%d", app.salesURL, page, rowsPerPage)
	req, err := app.newGetRequest(ctx, r, url)
	if err != nil {
		app.serverError(w, err)
		return
	}

	span.AddEvent("Lookup Products")
	span.SetAttributes(attribute.String("url", url))

	// Client.Do will handle the context level timeout.
	client := newClient()
	resp, err := client.Do(req)
	if err != nil {
		app.serverError(w, err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusUnauthorized {
		http.Redirect(w, r, "/user/login", http.StatusSeeOther)
		return
	}

	// Decode json response into products.
	var products []product.Product
	if err := json.NewDecoder(resp.Body).Decode(&products); err != nil {
		app.serverError(w, err)
		return
	}

	span.AddEvent("Render Home Page")

	if err != nil {
		app.serverError(w, err)
		return
	}

	app.render(w, r, "home.page.tmpl", &templateData{
		Path:     "/product",
		Products: products,
	})
}

func (app *application) about(w http.ResponseWriter, r *http.Request) {
	app.render(w, r, "about.page.tmpl", &templateData{})
}

func (app *application) showProduct(w http.ResponseWriter, r *http.Request) {

	ctx, span := otel.Tracer(name).Start(r.Context(), "showProduct")
	defer span.End()

	// Create a context with a timeout of 1 second.
	ctx, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()

	id := r.URL.Query().Get(":id")
	url := fmt.Sprintf("%s/products/%s", app.salesURL, id)
	req, err := app.newGetRequest(ctx, r, url)
	if err != nil {
		app.serverError(w, err)
		return
	}

	// Client.Do will handle the context level timeout.
	client := newClient()
	resp, err := client.Do(req)
	if err != nil {
		app.serverError(w, err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		app.clientError(w, resp.StatusCode)
		return
	}
	// Decode json response into a product.
	var product product.Product
	if err := json.NewDecoder(resp.Body).Decode(&product); err != nil {
		app.serverError(w, err)
		return
	}

	if err != nil {
		app.serverError(w, err)
		return
	}

	app.render(w, r, "show.page.tmpl", &templateData{
		Product: &product,
	})
}

func (app *application) loginUserForm(w http.ResponseWriter, r *http.Request) {
	app.render(w, r, "login.page.tmpl", &templateData{
		Form: forms.New(nil),
	})
}

// loginUser checks the provided credentials and redirects the client
// to the requested path
func (app *application) loginUser(w http.ResponseWriter, r *http.Request) {

	ctx, span := otel.Tracer(name).Start(r.Context(), "loginUser")
	defer span.End()

	err := r.ParseForm()
	if err != nil {
		app.clientError(w, http.StatusBadRequest)
		return
	}

	// Initialize a form struct using form data.
	form := forms.New(r.PostForm)

	// Create a context with a timeout of 1 second.
	ctx, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()

	url := fmt.Sprintf("%s/users/token/%s", app.salesURL, app.keyID)
	req, err := app.newGetRequest(ctx, r, url)
	if err != nil {
		app.serverError(w, err)
		return
	}
	req.SetBasicAuth(form.Get("email"), form.Get("password"))

	// Login with provided credentials.
	// Client.Do will handle the context level timeout.
	client := newClient()
	resp, err := client.Do(req)
	if err != nil {
		app.serverError(w, err)
		return
	}

	// Close the response body on the return.
	defer resp.Body.Close()

	// If the credentials are not valid, add a generic error message to the
	// form failures map and re-display the login page.
	if resp.StatusCode != http.StatusOK {
		form.Errors.Add("generic", "Email or Password is incorrect")
		app.render(w, r, "login.page.tmpl", &templateData{Form: form})
		return
	}

	// Extract user ID from the json web token
	var tkn struct {
		Token string `json:"token"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&tkn); err != nil {
		app.serverError(w, err)
		return
	}

	var po []jwt.ParserOption
	po = append(po, jwt.WithValidMethods([]string{"RS256"}))
	parser := jwt.NewParser(po...)
	var claims jwt.StandardClaims
	_, _, err = parser.ParseUnverified(tkn.Token, &claims)
	if err != nil {
		app.serverError(w, err)
		return
	}

	// Add the ID of the current user to the session data (user loged in)
	app.session.Put(r, "authenticatedUserID", claims.Subject)
	app.session.Put(r, "jsonWebToken", tkn.Token)

	if err != nil {
		app.serverError(w, err)
		return
	}

	// Pop the captured path from the session data.
	path := app.session.PopString(r, "redirectPathAfterLogin")
	if path != "" {
		http.Redirect(w, r, path, http.StatusSeeOther)
		return
	}

	// Redirect the user to the root page.
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func (app *application) logoutUser(w http.ResponseWriter, r *http.Request) {
	// remove authenticatedUserID from the session data (user logged out)
	app.session.Remove(r, "authenticatedUserID")
	// add flash message to the user session
	app.session.Put(r, "flash", "You've been logged out successfully!")
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func (app *application) userProfile(w http.ResponseWriter, r *http.Request) {

	ctx, span := otel.Tracer(name).Start(r.Context(), "userprofile")
	defer span.End()

	// Create a context with a timeout of 1 second.
	ctx, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()

	// get user ID from session data
	userID := app.session.GetString(r, "authenticatedUserID")
	url := fmt.Sprintf("%s/users/%s", app.salesURL, userID)
	req, err := app.newGetRequest(ctx, r, url)
	if err != nil {
		app.serverError(w, err)
		return
	}

	// Client.Do will handle the context level timeout.
	client := newClient()
	resp, err := client.Do(req)
	if err != nil {
		app.serverError(w, err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		app.clientError(w, resp.StatusCode)
		return
	}

	// Decode json response into a user.
	var u user.User
	if err := json.NewDecoder(resp.Body).Decode(&u); err != nil {
		app.serverError(w, err)
		return
	}

	if err != nil {
		app.serverError(w, err)
		return
	}

	app.render(w, r, "profile.page.tmpl", &templateData{
		User: &u,
	})
}
