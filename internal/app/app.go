// Package app configures and runs application.
package app

import (
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/banjuer/kompanion/config"
	"github.com/banjuer/kompanion/internal/auth"
	"github.com/banjuer/kompanion/internal/bookmeta"
	"github.com/banjuer/kompanion/internal/controller/http/opds"
	v1 "github.com/banjuer/kompanion/internal/controller/http/v1"
	"github.com/banjuer/kompanion/internal/controller/http/web"
	"github.com/banjuer/kompanion/internal/controller/http/webdav"
	"github.com/banjuer/kompanion/internal/library"
	"github.com/banjuer/kompanion/internal/stats"
	"github.com/banjuer/kompanion/internal/storage"
	"github.com/banjuer/kompanion/internal/sync"
	"github.com/banjuer/kompanion/pkg/httpserver"
	"github.com/banjuer/kompanion/pkg/logger"
	"github.com/banjuer/kompanion/pkg/postgres"
)

// Run creates objects via constructors.
func Run(cfg *config.Config) {
	l := logger.New(cfg.Log.Level)

	// Repository
	pg, err := postgres.New(cfg.PG.URL, postgres.MaxPoolSize(cfg.PG.PoolMax))
	if err != nil {
		l.Fatal(fmt.Errorf("app - Run - postgres.New: %w", err))
	}
	defer pg.Close()

	bookStorage, err := storage.NewStorage(cfg.BookStorage.Type, cfg.BookStorage.Path, pg)
	if err != nil {
		l.Fatal(fmt.Errorf("app - Run - storage.NewStorage: %w", err))
	}

	// Use case
	var repo auth.UserRepo
	switch cfg.Auth.Storage {
	case "memory":
		repo = auth.NewMemoryUserRepo()
	case "postgres":
		repo = auth.NewUserDatabaseRepo(pg)
	default:
		l.Fatal(fmt.Errorf("app - Run - unknown storage: %s", cfg.Auth.Storage))
	}
	authService := auth.InitAuthService(
		repo,
		cfg.Auth.Username,
		cfg.Auth.Password,
	)
	progress := sync.NewProgressSync(sync.NewProgressDatabaseRepo(pg))
	metadataProvider := newMetadataProvider(cfg, l)
	shelf := library.NewBookShelf(bookStorage, library.NewBookDatabaseRepo(pg), l, metadataProvider)
	rs := stats.NewKOReaderPGStats(pg)

	// HTTP Server
	handler := gin.New()
	web.NewRouter(handler, l, authService, progress, shelf, rs, cfg.Version)
	v1.NewRouter(handler, l, authService, progress, shelf)
	opds.NewRouter(handler, l, authService, progress, shelf)
	webdav.NewRouter(handler, authService, l, rs, shelf)
	httpServer := httpserver.New(handler, httpserver.Port(cfg.HTTP.Port))

	// Waiting signal
	interrupt := make(chan os.Signal, 1)
	signal.Notify(interrupt, os.Interrupt, syscall.SIGTERM)

	select {
	case s := <-interrupt:
		l.Info("app - Run - signal: " + s.String())
	case err = <-httpServer.Notify():
		l.Error(fmt.Errorf("app - Run - httpServer.Notify: %w", err))
	}

	// Shutdown
	err = httpServer.Shutdown()
	if err != nil {
		l.Error(fmt.Errorf("app - Run - httpServer.Shutdown: %w", err))
	}
}

func newMetadataProvider(cfg *config.Config, l logger.Interface) bookmeta.Provider {
	if strings.ToLower(cfg.Metadata.Provider) != "douban" {
		return nil
	}

	var cookieSource bookmeta.CookieSource
	switch {
	case strings.TrimSpace(cfg.Metadata.DoubanCookie) != "":
		cookieSource = bookmeta.NewStaticCookieSource(cfg.Metadata.DoubanCookie)
	case cfg.Metadata.CookieCloudURL != "" && cfg.Metadata.CookieCloudUUID != "" && cfg.Metadata.CookieCloudPassword != "":
		cookieSource = bookmeta.NewCookieCloudSource(
			cfg.Metadata.CookieCloudURL,
			cfg.Metadata.CookieCloudUUID,
			cfg.Metadata.CookieCloudPassword,
			&http.Client{Timeout: 8 * time.Second},
		)
	default:
		l.Warn("app - Run - douban metadata provider enabled without cookie configuration")
		return nil
	}

	provider := bookmeta.NewDoubanProvider(cookieSource, &http.Client{Timeout: 8 * time.Second})
	provider.SetCookieDomain(cfg.Metadata.CookieCloudDomain)
	return provider
}
