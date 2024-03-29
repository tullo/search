package main

import (
	"context"
	"crypto/tls"
	"encoding/base64"
	"fmt"
	"html/template"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/golangcollege/sessions"
	"github.com/pkg/errors"
	"github.com/tullo/conf"
	"github.com/tullo/search/tracer"
)

// build is the git version of this application. It is set using build flags in the makefile.
var build = "develop"

// the key must be unexported type to avoid collisions
type contextKey string

const contextKeyIsAuthenticated = contextKey("isAuthenticated")

// define the interfaces inline to keep the code simple
type application struct {
	debug         bool
	debugURL      string
	keyID         string
	log           *log.Logger
	salesURL      string
	session       *sessions.Session
	shutdown      chan os.Signal
	templateCache map[string]*template.Template
	useTLS        bool
}

// SignalShutdown is used to gracefully shutdown the app when an integrity
// issue is identified.
func (a *application) SignalShutdown() {
	a.shutdown <- syscall.SIGTERM
}

func main() {
	log := log.New(os.Stdout, "SEARCH : ", log.LstdFlags|log.Lmicroseconds|log.Lshortfile)

	if err := run(log); err != nil {
		log.Printf("error: %s", err)
		os.Exit(1)
	}
}

func run(log *log.Logger) error {

	// =========================================================================
	// Configuration

	// session secret (must be 32 bytes long) is used to encrypt and authenticate session cookies
	// e.g. 'openssl rand -base64 32'

	var cfg struct {
		conf.Version
		Debug struct {
			BaseURL string `conf:"default:http://0.0.0.0:4000/debug"`
		}
		IdentityProvider struct {
			KeyID string `conf:"default:54bb2165-71e1-41a6-af3e-7da4a0e1e2c1"`
		}
		Web struct {
			Host            string        `conf:"default::4200"`
			DebugMode       bool          `conf:"default:false"`
			EnableTLS       bool          `conf:"default:false"`
			SessionSecret   string        `conf:"noprint"`
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
		Zipkin struct {
			ReporterURI string  `conf:"default:http://zipkin:9411/api/v2/spans"`
			ServiceName string  `conf:"default:search"`
			Probability float64 `conf:"default:0.05"`
		}
		Args conf.Args
	}
	cfg.Version.Version = build
	cfg.Version.Description = "copyright information here"

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

	// =========================================================================
	// Start Web Application

	log.Printf("Initializing Application: version %q\n", build)

	out, err := conf.String(&cfg)
	if err != nil {
		return errors.Wrap(err, "generating config for output")
	}
	log.Printf("Config :\n%v\n", out)

	// initialize template cache
	templateCache, err := newTemplateCache("./ui/html/")
	if err != nil {
		log.Fatal(err)
	}

	decoded, err := base64.StdEncoding.DecodeString(cfg.Web.SessionSecret)
	if err != nil {
		return errors.Wrap(err, "decoding session secret")
	}
	if len(decoded) != 32 {
		return errors.New("session secret must be exactly 32 bytes long")
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
		debugURL:      cfg.Debug.BaseURL,
		keyID:         cfg.IdentityProvider.KeyID,
		log:           log,
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
		Handler:      app.routes(),
		TLSConfig:    tlsConfig,
		IdleTimeout:  cfg.Web.IdleTimeout,
		ReadTimeout:  cfg.Web.ReadTimeout,
		WriteTimeout: cfg.Web.WriteTimeout,
	}

	// =========================================================================
	// Start Tracing Support

	log.Println("Initializing zipkin tracing support")

	shutdownTP, err := tracer.Init(cfg.Zipkin.ServiceName, cfg.Zipkin.ReporterURI, cfg.Zipkin.Probability, log)
	if err != nil {
		return errors.Wrap(err, "starting tracer")
	}

	defer func() {
		if err := shutdownTP(context.Background()); err != nil {
			log.Fatal("failed to shutdown TracerProvider: %w", err)
		}
	}()

	// Make a channel to listen for errors coming from the listener. Use a
	// buffered channel so the goroutine can exit if we don't collect this error.
	serverErrors := make(chan error, 1)

	// Start the application listening for requests.
	go func() {
		var b strings.Builder
		if cfg.Web.Host[:1] == ":" {
			fmt.Fprintf(&b, "%s%s/", getOutboundIP(), cfg.Web.Host)
		} else {
			fmt.Fprintf(&b, "%s/", cfg.Web.Host)
		}

		if app.useTLS {
			log.Printf("Starting server @ https://%s", b.String())
			serverErrors <- srv.ListenAndServeTLS("./tls/localhost/cert.pem", "./tls/localhost/key.pem")
			return
		}

		log.Printf("Starting server @ http://%s", b.String())
		serverErrors <- srv.ListenAndServe()
	}()

	// =========================================================================
	// Shutdown

	// Blocking main and waiting for shutdown.
	select {
	case err := <-serverErrors:
		return errors.Wrap(err, "server error")

	case sig := <-shutdown:
		log.Printf("main : %v : Start shutdown", sig)

		// Give outstanding requests a deadline for completion.
		ctx, cancel := context.WithTimeout(context.Background(), cfg.Web.ShutdownTimeout)
		defer cancel()

		// Trigger graceful shutdown of the server, listeners.
		err := srv.Shutdown(ctx)
		if err != nil {
			log.Printf("Graceful shutdown did not complete in %v : %v", cfg.Web.ShutdownTimeout, err)
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

// Get the preferred outbound IP address of this machine.
func getOutboundIP() net.IP {
	conn, err := net.Dial("udp", "1.1.1.1:80")
	if err != nil {
		log.Fatal(err)
	}
	defer conn.Close()

	localAddr := conn.LocalAddr().(*net.UDPAddr)

	return localAddr.IP
}
