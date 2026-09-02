// Command server is apps/server: the composition root for the billing
// platform's HTTP API. It wires concrete repositories and adapters into
// application services and mounts HTTP handlers — it contains no business
// logic itself (docs/architecture.md §2).
package main

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/go-chi/chi/v5"

	catalogueapp "billing-platform/internal/modules/catalogue/app"
	cataloguehttp "billing-platform/internal/modules/catalogue/httpapi"
	cataloguepg "billing-platform/internal/modules/catalogue/pg"
	contactsapp "billing-platform/internal/modules/contacts/app"
	contactshttp "billing-platform/internal/modules/contacts/httpapi"
	contactspg "billing-platform/internal/modules/contacts/pg"
	identityapp "billing-platform/internal/modules/identity/app"
	identityhttp "billing-platform/internal/modules/identity/httpapi"
	identitypg "billing-platform/internal/modules/identity/pg"
	inventoryapp "billing-platform/internal/modules/inventory/app"
	inventoryhttp "billing-platform/internal/modules/inventory/httpapi"
	inventorypg "billing-platform/internal/modules/inventory/pg"
	orgapp "billing-platform/internal/modules/organisation/app"
	orghttp "billing-platform/internal/modules/organisation/httpapi"
	orgpg "billing-platform/internal/modules/organisation/pg"
	pricingapp "billing-platform/internal/modules/pricing/app"
	pricinghttp "billing-platform/internal/modules/pricing/httpapi"
	pricingpg "billing-platform/internal/modules/pricing/pg"
	purchasesapp "billing-platform/internal/modules/purchases/app"
	purchaseshttp "billing-platform/internal/modules/purchases/httpapi"
	purchasespg "billing-platform/internal/modules/purchases/pg"
	"billing-platform/internal/platform/audit"
	"billing-platform/internal/platform/config"
	appcrypto "billing-platform/internal/platform/crypto"
	"billing-platform/internal/platform/database"
	httpx "billing-platform/internal/platform/http"
	"billing-platform/internal/platform/logging"
	"billing-platform/internal/platform/observability"
	"billing-platform/internal/platform/permissions"
	"billing-platform/migrations"
)

func main() {
	if err := run(); err != nil {
		slog.Error("server exited with error", "error", err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}

	logger := logging.NewDefault(cfg.Logging.Level)
	slog.SetDefault(logger)

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	shutdownTracing, err := observability.Setup(ctx, observability.Config{
		ServiceName:  cfg.Observability.ServiceName,
		OTLPEndpoint: cfg.Observability.OTLPEndpoint,
	})
	if err != nil {
		return err
	}
	defer func() {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = shutdownTracing(shutdownCtx)
	}()

	pool, err := database.NewPool(ctx, database.Config{DSN: cfg.Database.DSN, MaxConns: cfg.Database.MaxConns})
	if err != nil {
		return err
	}
	defer pool.Close()

	if cfg.Database.AutoMigrate {
		logger.Info("applying database migrations")
		if err := database.Migrate(cfg.Database.DSN, migrations.FS); err != nil {
			return err
		}
	}
	if err := pool.WarnIfRuntimeRoleOwnsTenantTables(ctx, logger); err != nil && !errors.Is(err, context.Canceled) {
		logger.Warn("startup RLS-ownership check failed", "error", err)
	}

	hasher, err := appcrypto.NewPasswordHasher(appcrypto.PasswordParams{
		MemoryKiB:   cfg.Argon2.MemoryKiB,
		Iterations:  cfg.Argon2.Iterations,
		Parallelism: cfg.Argon2.Parallelism,
		SaltLength:  cfg.Argon2.SaltLength,
		KeyLength:   cfg.Argon2.KeyLength,
	})
	if err != nil {
		return err
	}

	aeadKey, err := loadOrGenerateAEADKey(logger)
	if err != nil {
		return err
	}
	aead, err := appcrypto.NewAEAD(aeadKey)
	if err != nil {
		return err
	}

	auditRecorder := audit.NewPGRecorder(pool)
	permissionsChecker := permissions.NewChecker(permissions.NewPGStore(pool), pool)

	orgSvc := orgapp.NewService(
		pool,
		orgpg.NewOrganisationRepo(pool),
		orgpg.NewLegalEntityRepo(pool),
		orgpg.NewBranchRepo(pool),
		orgpg.NewWarehouseRepo(pool),
		permissionsChecker,
		auditRecorder,
	)

	catalogueSvc := catalogueapp.NewService(
		pool,
		cataloguepg.NewUnitOfMeasureRepo(pool),
		cataloguepg.NewUnitConversionRepo(pool),
		cataloguepg.NewCategoryRepo(pool),
		cataloguepg.NewBrandRepo(pool),
		cataloguepg.NewProductRepo(pool),
		cataloguepg.NewProductVariantRepo(pool),
		cataloguepg.NewBarcodeRepo(pool),
		permissionsChecker,
		auditRecorder,
	)

	contactsSvc := contactsapp.NewService(
		pool,
		contactspg.NewPartyRepo(pool),
		contactspg.NewAddressRepo(pool),
		contactspg.NewTaxRegistrationRepo(pool),
		permissionsChecker,
		auditRecorder,
	)

	pricingSvc := pricingapp.NewService(
		pool,
		pricingpg.NewPriceListRepo(pool),
		pricingpg.NewPriceListItemRepo(pool),
		permissionsChecker,
		auditRecorder,
	)

	inventorySvc := inventoryapp.NewService(
		pool,
		inventorypg.NewStockMovementRepo(pool),
		inventorypg.NewStockBalanceRepo(pool),
		inventorypg.NewStockReservationRepo(pool),
		inventorypg.NewStockBatchRepo(pool),
		inventorypg.NewSerialNumberRepo(pool),
		inventorypg.NewStockPolicyRepo(pool),
		inventorypg.NewStockTransferRepo(pool),
		inventorypg.NewStockAdjustmentRepo(pool),
		cataloguepg.NewProductVariantRepo(pool),
		cataloguepg.NewProductRepo(pool),
		cataloguepg.NewUnitConversionRepo(pool),
		permissionsChecker,
		auditRecorder,
	)

	purchasesSvc := purchasesapp.NewService(
		pool,
		purchasespg.NewDocumentRepo(pool),
		purchasespg.NewDocumentLineRepo(pool),
		inventorySvc,
		permissionsChecker,
		auditRecorder,
	)

	identitySvc := identityapp.NewService(
		pool,
		identitypg.NewUserRepo(pool),
		identitypg.NewSessionRepo(pool),
		identitypg.NewPasswordResetRepo(pool),
		identitypg.NewMFARepo(pool),
		identitypg.NewRoleRepo(pool),
		orgSvc,
		hasher,
		aead,
		auditRecorder,
		identityapp.SessionPolicy{
			IdleTimeout:     cfg.Session.IdleTimeout,
			AbsoluteTimeout: cfg.Session.AbsoluteTimeout,
		},
	)

	router := httpx.NewRouter(httpx.RouterConfig{AllowedOrigins: cfg.Server.AllowedOrigins, Logger: logger})
	httpx.MountReady(router, pool)

	bootstrapEnabled := os.Getenv("ENABLE_BOOTSTRAP") == "true"
	if bootstrapEnabled {
		logger.Warn("POST /api/v1/auth/bootstrap is enabled — this endpoint creates a new organisation with " +
			"no permission check by design (see identity/app.Service.Bootstrap). Disable ENABLE_BOOTSTRAP once " +
			"initial setup is done.")
	}

	router.Route("/api/v1", func(r chi.Router) {
		identityHandlers := identityhttp.NewHandlers(identitySvc, cfg.Session.CookieName, cfg.Session.Secure)
		identityHandlers.Mount(r, bootstrapEnabled)

		r.Group(func(r chi.Router) {
			r.Use(identityhttp.RequireAuth(identitySvc, cfg.Session.CookieName))
			orghttp.NewHandlers(orgSvc).Mount(r)
			cataloguehttp.NewHandlers(catalogueSvc).Mount(r)
			contactshttp.NewHandlers(contactsSvc).Mount(r)
			pricinghttp.NewHandlers(pricingSvc).Mount(r)
			inventoryhttp.NewHandlers(inventorySvc).Mount(r)
			purchaseshttp.NewHandlers(purchasesSvc).Mount(r)
		})
	})

	server := &http.Server{
		Addr:              ":" + strconv.Itoa(cfg.Server.HTTPPort),
		Handler:           router,
		ReadHeaderTimeout: 10 * time.Second,
	}

	serveErr := make(chan error, 1)
	go func() {
		logger.Info("listening", "port", cfg.Server.HTTPPort)
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			serveErr <- err
		}
	}()

	select {
	case <-ctx.Done():
		logger.Info("shutdown signal received")
	case err := <-serveErr:
		return err
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), cfg.Server.ShutdownTimeout)
	defer cancel()
	return server.Shutdown(shutdownCtx)
}

// loadOrGenerateAEADKey reads a base64-encoded 32-byte key from
// AEAD_ENCRYPTION_KEY. In production this must be set from secrets
// management (brief §60) — generating an ephemeral key is only
// acceptable for local development, where losing already-encrypted MFA
// secrets on restart is a non-issue, and this path logs loudly so it's
// never silently relied on in a real deployment.
func loadOrGenerateAEADKey(logger *slog.Logger) ([]byte, error) {
	if encoded := os.Getenv("AEAD_ENCRYPTION_KEY"); encoded != "" {
		key, err := base64.StdEncoding.DecodeString(encoded)
		if err != nil {
			return nil, errors.New("config: AEAD_ENCRYPTION_KEY is not valid base64")
		}
		if len(key) != 32 {
			return nil, errors.New("config: AEAD_ENCRYPTION_KEY must decode to exactly 32 bytes")
		}
		return key, nil
	}
	logger.Warn("AEAD_ENCRYPTION_KEY not set — generating an EPHEMERAL key for this process only. " +
		"Any MFA secret encrypted with it becomes unreadable on restart. Set AEAD_ENCRYPTION_KEY " +
		"(32 random bytes, base64-encoded) before running this in production.")
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		return nil, err
	}
	return key, nil
}
