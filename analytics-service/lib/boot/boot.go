// Package boot wires every singleton, starts the consumer, serves HTTP, and
// runs graceful shutdown. main.go calls Run() and exits.
//
// Wiring lives here (not in main.go) so adding a module means editing one
// function, and main.go stays a 5-line process entry point.
package boot

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/go-chi/chi/v5"
	chimw "github.com/go-chi/chi/v5/middleware"
	"github.com/go-playground/validator/v10"

	analyticsctrl "github.com/quickbite/analytics-service/app/analytics/controller"
	"github.com/quickbite/analytics-service/app/analytics/eventhandlers"
	"github.com/quickbite/analytics-service/app/analytics/repository"
	"github.com/quickbite/analytics-service/app/analytics/service"
	"github.com/quickbite/analytics-service/lib/config"
	"github.com/quickbite/analytics-service/lib/coreclient"
	"github.com/quickbite/analytics-service/lib/coreevents"
	apphttp "github.com/quickbite/analytics-service/lib/http"
	"github.com/quickbite/analytics-service/lib/logger"
	appmw "github.com/quickbite/analytics-service/lib/middleware"
	"github.com/quickbite/analytics-service/lib/rbac"
	pkgmessaging "github.com/quickbite/analytics-service/pkg/messaging"
	pkgmongo "github.com/quickbite/analytics-service/pkg/mongo"
)

// Run boots the whole service. Blocks until SIGINT/SIGTERM, then shuts down.
func Run() error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("config: %w", err)
	}
	log := logger.New(cfg.LogLevel)
	log.Info("starting analytics-service", slog.String("env", cfg.AppEnv), slog.Int("port", cfg.HTTP.Port))

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// Mongo + index sync.
	bootCtx, bootCancel := context.WithTimeout(ctx, 10*time.Second)
	defer bootCancel()
	db, mongoClose, err := pkgmongo.Connect(bootCtx, cfg.Mongo.URI, cfg.Mongo.DB)
	if err != nil {
		return fmt.Errorf("mongo: %w", err)
	}
	if err := repository.EnsureIndexes(bootCtx, db); err != nil {
		return fmt.Errorf("ensure indexes: %w", err)
	}
	log.Info("mongo connected", slog.String("db", cfg.Mongo.DB))

	// Broker — best-effort at boot; consumer.Subscribe retries internally.
	broker := pkgmessaging.NewAMQPBroker(cfg.AMQP.URL)
	if err := broker.Connect(bootCtx); err != nil {
		log.Warn("rabbit not reachable at boot", slog.String("error", err.Error()))
	} else {
		log.Info("rabbit connected")
	}

	// App wiring — one repo line per aggregate, one service, one controller.
	restaurantRepo := repository.NewAggRestaurantDayRepo(db)
	branchRepo := repository.NewAggBranchDayRepo(db)
	productRepo := repository.NewAggProductDayRepo(db)
	platformRepo := repository.NewAggPlatformDayRepo(db)
	eventIDs := repository.NewEventIDsRepo(db)
	svc := service.NewAnalyticsService(restaurantRepo, branchRepo, productRepo, platformRepo)
	v := validator.New(validator.WithRequiredStructEnabled())
	ctrl := analyticsctrl.NewAnalyticsController(svc, v)
	core := coreclient.New(cfg.Core.BaseURL, cfg.Core.APIKey)
	rbacCache := rbac.NewCache(cfg.RBAC.CacheTTL(), core.GetPermissionsForRole)

	// Consumer — order.events (order/payment rollups).
	orderOpts := pkgmessaging.ConsumerOpts{
		Exchange: cfg.AMQP.Exchange, Queue: cfg.AMQP.Queue, BindingKeys: cfg.AMQP.Bindings,
		DLX: cfg.AMQP.DLX, DLQ: cfg.AMQP.DLQ, Prefetch: cfg.AMQP.Prefetch,
	}
	if err := coreevents.Start(ctx, log, broker, orderOpts, eventhandlers.For(svc), eventIDs); err != nil {
		log.Warn("order-events consumer start failed", slog.String("error", err.Error()))
	}

	// Consumer — core.events (RBAC cache invalidation only).
	coreOpts := pkgmessaging.ConsumerOpts{
		Exchange: cfg.AMQP.CoreExchange, Queue: cfg.AMQP.CoreQueue, BindingKeys: cfg.AMQP.CoreBindings,
		DLX: cfg.AMQP.CoreDLX, DLQ: cfg.AMQP.CoreDLQ, Prefetch: cfg.AMQP.Prefetch,
	}
	coreHandlers := map[string]coreevents.EventHandler{
		"rbac.permissions_changed": rbac.InvalidationHandler(rbacCache, log),
	}
	if err := coreevents.Start(ctx, log, broker, coreOpts, coreHandlers, eventIDs); err != nil {
		log.Warn("core-events consumer start failed", slog.String("error", err.Error()))
	}

	// HTTP.
	r := chi.NewRouter()
	r.Use(chimw.Recoverer)
	r.Use(appmw.Correlation)
	r.Use(appmw.AccessLog(log))
	r.Get("/health", func(w http.ResponseWriter, _ *http.Request) {
		apphttp.SendSuccess(w, http.StatusOK, map[string]string{"status": "ok"})
	})
	r.Route("/api/v1", func(r chi.Router) {
		analyticsctrl.Mount(r, log, cfg, rbacCache, ctrl)
	})

	srv := &http.Server{
		Addr:              fmt.Sprintf(":%d", cfg.HTTP.Port),
		Handler:           r,
		ReadHeaderTimeout: 5 * time.Second,
	}
	serveErr := make(chan error, 1)
	go func() {
		log.Info("http listening", slog.String("addr", srv.Addr))
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			serveErr <- err
		}
	}()

	select {
	case <-ctx.Done():
		log.Info("shutdown signal received")
	case err := <-serveErr:
		log.Error("http server crashed", slog.String("error", err.Error()))
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_ = srv.Shutdown(shutdownCtx)
	_ = broker.Close()
	_ = mongoClose(shutdownCtx)
	log.Info("shutdown complete")
	return nil
}
