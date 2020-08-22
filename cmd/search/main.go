package main

import (
	"context"
	"crypto/tls"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/ardanlabs/conf"
	"github.com/golangcollege/sessions"
	"github.com/pkg/errors"
)

// build is the git version of this application. It is set using build flags in the makefile.
var build = "develop"

// the key must be unexported type to avoid collisions
type contextKey string

const contextKeyIsAuthenticated = contextKey("isAuthenticated")

// define the interfaces inline to keep the code simple
type application struct {
	debug         bool
	errorLog      *log.Logger
	infoLog       *log.Logger
	salesURL      string
	session       *sessions.Session
	shutdown      chan os.Signal
	templateCache map[string]*template.Template
	useTLS        bool
	users         interface {
		Authenticate(string, string) (string, error)
	}
}

// SignalShutdown is used to gracefully shutdown the app when an integrity
// issue is identified.
func (a *application) SignalShutdown() {
	a.shutdown <- syscall.SIGTERM
}

func main() {
	if err := run(); err != nil {
		log.Printf("error: %s", err)
		os.Exit(1)
	}
}

func run() error {

	// =========================================================================
	// Configuration

	// session secret (should be 32 bytes long) is used to encrypt and authenticate session cookies
	// e.g. 'openssl rand -base64 32'

	var cfg struct {
		conf.Version
		Web struct {
			Host            string        `conf:"default::4200"`
			DebugMode       bool          `conf:"default:false"`
			EnableTLS       bool          `conf:"default:false"`
			SessionSecret   string        `conf:"noprint,default:M+ZrbJjvTLvXOvihe+Rjlr/ccfGjmFReGtLcV7gSufg="`
			IdleTimeout     time.Duration `conf:"default:1m"`
			ReadTimeout     time.Duration `conf:"default:5s"`
			WriteTimeout    time.Duration `conf:"default:5s"`
			ShutdownTimeout time.Duration `conf:"default:5s"`
		}
		Sales struct {
			BaseURL         string        `conf:"default:http://0.0.0.0:3000/v1"`
			IdleTimeout     time.Duration `conf:"default:1m"`
			ReadTimeout     time.Duration `conf:"default:5s"`
			WriteTimeout    time.Duration `conf:"default:5s"`
			ShutdownTimeout time.Duration `conf:"default:5s"`
		}
		Args conf.Args
	}
	cfg.Version.SVN = build
	cfg.Version.Desc = "copyright information here"

	if err := conf.Parse(os.Args[1:], "SEARCH", &cfg); err != nil {
		if err == conf.ErrHelpWanted {
			usage, err := conf.Usage("SEARCH", &cfg)
			if err != nil {
				return errors.Wrap(err, "generating usage")
			}
			fmt.Println(usage)
			return nil
		}
		return errors.Wrap(err, "error: parsing config")
	}

	if len(cfg.Web.SessionSecret) == 0 {
		return errors.New("session secret cannot be empty")
	}

	// =========================================================================
	// Start Web Application

	infoLog := log.New(os.Stdout, "INFO\t", log.Ldate|log.Ltime)
	errorLog := log.New(os.Stderr, "ERROR\t", log.Ldate|log.Ltime|log.Lshortfile)

	infoLog.Printf("main: Started : Application initializing : version %q", build)
	defer infoLog.Println("main: Completed")

	out, err := conf.String(&cfg)
	if err != nil {
		return errors.Wrap(err, "generating config for output")
	}
	infoLog.Printf("main: Config :\n%v\n", out)

	// initialize template cache
	templateCache, err := newTemplateCache("./ui/html/")
	if err != nil {
		errorLog.Fatal(err)
	}

	// sessions expire after 12 hours
	session := sessions.New([]byte(cfg.Web.SessionSecret))
	session.Lifetime = 12 * time.Hour
	// set the secure flag on session cookies and
	// serve all requests over https in production environment
	session.Secure = true
	session.SameSite = http.SameSiteStrictMode

	// make a channel to listen for an interrupt or terminate signal from the OS.
	// use a buffered channel because the signal package requires it.
	shutdown := make(chan os.Signal, 1)
	signal.Notify(shutdown, os.Interrupt, syscall.SIGTERM)

	app := &application{
		debug:         cfg.Web.DebugMode,
		errorLog:      errorLog,
		infoLog:       infoLog,
		salesURL:      cfg.Sales.BaseURL,
		session:       session,
		shutdown:      shutdown,
		templateCache: templateCache,
		useTLS:        cfg.Web.EnableTLS,
	}

	// use Go’s favored cipher suites (support for forward secrecy)
	// and elliptic curves that are performant under heavy loads
	tlsConfig := &tls.Config{
		PreferServerCipherSuites: true,
		CurvePreferences:         []tls.CurveID{tls.X25519, tls.CurveP256},
	}

	srv := &http.Server{
		Addr:         cfg.Web.Host,
		ErrorLog:     errorLog,
		Handler:      app.routes(),
		TLSConfig:    tlsConfig,
		IdleTimeout:  cfg.Web.IdleTimeout,
		ReadTimeout:  cfg.Web.ReadTimeout,
		WriteTimeout: cfg.Web.WriteTimeout,
	}

	// Make a channel to listen for errors coming from the listener. Use a
	// buffered channel so the goroutine can exit if we don't collect this error.
	serverErrors := make(chan error, 1)

	// Start the application listening for requests.
	go func() {
		infoLog.Printf("Starting server on %s", cfg.Web.Host)
		if app.useTLS {
			serverErrors <- srv.ListenAndServeTLS("./tls/localhost/cert.pem", "./tls/localhost/key.pem")
		}
		serverErrors <- srv.ListenAndServe()
	}()

	// =========================================================================
	// Shutdown

	// Blocking main and waiting for shutdown.
	select {
	case err := <-serverErrors:
		return errors.Wrap(err, "server error")

	case sig := <-shutdown:
		infoLog.Printf("main : %v : Start shutdown", sig)

		// Give outstanding requests a deadline for completion.
		ctx, cancel := context.WithTimeout(context.Background(), cfg.Web.ShutdownTimeout)
		defer cancel()

		// Trigger graceful shutdown of the server, listeners.
		err := srv.Shutdown(ctx)
		if err != nil {
			infoLog.Printf("main : Graceful shutdown did not complete in %v : %v", cfg.Web.ShutdownTimeout, err)
			err = srv.Close()
		}

		// Log the status of this shutdown.
		switch {
		case sig == syscall.SIGSTOP:
			return errors.New("integrity issue caused shutdown")
		case err != nil:
			return errors.Wrap(err, "could not stop server gracefully")
		}
	}

	return nil
}
