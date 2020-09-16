package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/dgrijalva/jwt-go"
	"github.com/tullo/search/internal/forms"
	"github.com/tullo/search/internal/product"
	"github.com/tullo/search/internal/user"
	"go.opentelemetry.io/otel/api/propagation"
	"go.opentelemetry.io/otel/api/trace"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/label"
)

func (app *application) home(w http.ResponseWriter, r *http.Request) {

	// Create a context with a timeout of 1 second.
	ctx, cancel := context.WithTimeout(r.Context(), time.Second)
	defer cancel()

	url := fmt.Sprintf("%s/products", app.salesURL)
	req, err := app.newGetRequest(ctx, r, url)
	if err != nil {
		app.serverError(w, err)
		return
	}

	var products []product.Product

	ctx, span := trace.SpanFromContext(ctx).Tracer().Start(ctx, "home")
	defer span.End()

	span.AddEvent(ctx, "Lookup Products")
	span.SetAttributes(label.String("url", url))

	b3 := trace.B3{InjectEncoding: trace.B3SingleHeader}
	props := propagation.New(propagation.WithInjectors(b3))
	propagation.InjectHTTP(ctx, props, req.Header)

	// Do will handle the context level timeout.
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		span.SetStatus(codes.Internal, "failure parsing")
		app.serverError(w, err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusUnauthorized {
		span.AddEvent(ctx, "Unauthorized Request")
		span.SetStatus(codes.Unauthenticated, "unauthorized")
		http.Redirect(w, r, "/user/login", http.StatusSeeOther)
		return
	}

	// Decode json response into products.
	span.AddEvent(ctx, "Decode JSON Response")
	if err := json.NewDecoder(resp.Body).Decode(&products); err != nil {
		span.SetStatus(codes.Internal, "failure decoding json")
		app.serverError(w, err)
		return
	}

	span.AddEvent(ctx, "Render Home Page")

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

	// Create a context with a timeout of 1 second.
	ctx, cancel := context.WithTimeout(r.Context(), time.Second)
	defer cancel()

	id := r.URL.Query().Get(":id")
	url := fmt.Sprintf("%s/products/%s", app.salesURL, id)
	req, err := app.newGetRequest(ctx, r, url)
	if err != nil {
		app.serverError(w, err)
		return
	}

	var product product.Product

	ctx, span := trace.SpanFromContext(ctx).Tracer().Start(ctx, "showProduct")
	defer span.End()

	span.AddEvent(ctx, "Show Product")
	span.SetAttributes(label.String("url", url))

	b3 := trace.B3{InjectEncoding: trace.B3SingleHeader}
	props := propagation.New(propagation.WithInjectors(b3))
	propagation.InjectHTTP(ctx, props, req.Header)

	// Do will handle the context level timeout.
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		span.SetStatus(codes.Internal, "failure sending request")
		app.serverError(w, err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		span.SetStatus(codes.FailedPrecondition, "unexpected status code")
		span.SetAttributes(label.Int("status code", resp.StatusCode))
		app.clientError(w, resp.StatusCode)
		return
	}
	// Decode json response into a product.
	if err := json.NewDecoder(resp.Body).Decode(&product); err != nil {
		span.SetStatus(codes.Internal, "failure decoding json")
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
	err := r.ParseForm()
	if err != nil {
		app.clientError(w, http.StatusBadRequest)
		return
	}

	// Initialize a form struct using form data.
	form := forms.New(r.PostForm)

	// Create a context with a timeout of 1 second.
	ctx, cancel := context.WithTimeout(r.Context(), time.Second)
	defer cancel()

	url := fmt.Sprintf("%s/users/token", app.salesURL)
	req, err := app.newGetRequest(ctx, r, url)
	if err != nil {
		app.serverError(w, err)
		return
	}
	req.SetBasicAuth(form.Get("email"), form.Get("password"))

	ctx, span := trace.SpanFromContext(ctx).Tracer().Start(ctx, "loginUser")
	defer span.End()

	span.AddEvent(ctx, "Login User")
	span.SetAttributes(label.String("url", url))

	b3 := trace.B3{InjectEncoding: trace.B3SingleHeader}
	props := propagation.New(propagation.WithInjectors(b3))
	propagation.InjectHTTP(ctx, props, req.Header)

	// Login with provided credentials.
	// Do will handle the context level timeout.
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		span.SetStatus(codes.Internal, "failure sending request")
		app.serverError(w, err)
		return
	}

	// Close the response body on the return.
	defer resp.Body.Close()

	// If the credentials are not valid, add a generic error message to the
	// form failures map and re-display the login page.
	if resp.StatusCode != http.StatusOK {
		span.SetStatus(codes.FailedPrecondition, "login failure: credentials not valid")
		form.Errors.Add("generic", "Email or Password is incorrect")
		app.render(w, r, "login.page.tmpl", &templateData{Form: form})
		return
	}

	// Extract user ID from the json web token
	var tkn struct {
		Token string `json:"token"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&tkn); err != nil {
		span.SetStatus(codes.Internal, "failure decoding json")
		app.serverError(w, err)
		return
	}

	parser := jwt.Parser{
		ValidMethods: []string{"RS256"},
	}
	var claims jwt.StandardClaims
	_, _, err = parser.ParseUnverified(tkn.Token, &claims)
	if err != nil {
		span.SetStatus(codes.Internal, "failure parsing jwt claims")
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

	// Create a context with a timeout of 1 second.
	ctx, cancel := context.WithTimeout(r.Context(), time.Second)
	defer cancel()

	// get user ID from session data
	userID := app.session.GetString(r, "authenticatedUserID")
	url := fmt.Sprintf("%s/users/%s", app.salesURL, userID)
	req, err := app.newGetRequest(ctx, r, url)
	if err != nil {
		app.serverError(w, err)
		return
	}
	var u user.User

	ctx, span := trace.SpanFromContext(ctx).Tracer().Start(ctx, "userprofile")
	defer span.End()

	span.AddEvent(ctx, "User Profile")
	span.SetAttributes(label.String("url", url))

	b3 := trace.B3{InjectEncoding: trace.B3SingleHeader}
	props := propagation.New(propagation.WithInjectors(b3))
	propagation.InjectHTTP(ctx, props, req.Header)

	// Do will handle the context level timeout.
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		span.SetStatus(codes.Internal, "failure sending request")
		app.serverError(w, err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		span.SetStatus(codes.FailedPrecondition, "unexpected status code")
		app.clientError(w, resp.StatusCode)
		return
	}

	// Decode json response into a user.
	if err := json.NewDecoder(resp.Body).Decode(&u); err != nil {
		span.SetStatus(codes.Internal, "failure decoding json")
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
