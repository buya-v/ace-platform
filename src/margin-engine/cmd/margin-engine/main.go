package main

import (
	"fmt"
	"log"
	"os"
	"os/signal"
	"strconv"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/garudax-platform/margin-engine/internal/collateral"
	"github.com/garudax-platform/margin-engine/internal/engine"
	"github.com/garudax-platform/margin-engine/internal/params"
	"github.com/garudax-platform/margin-engine/internal/server"
	"github.com/garudax-platform/margin-engine/internal/store"
	"github.com/garudax-platform/margin-engine/internal/types"
)

type seqIDGen struct {
	counter uint64
}

func (g *seqIDGen) NewID() string {
	n := atomic.AddUint64(&g.counter, 1)
	return fmt.Sprintf("mc-%d", n)
}

// inMemoryCollateral is a simple in-memory collateral source for dev/testing.
type inMemoryCollateral struct{}

func (c *inMemoryCollateral) GetCollateral(participantID string) types.Decimal {
	// Default zero collateral; production will use a real collateral service
	return types.DecimalZero()
}

func main() {
	log.Println("GarudaX Margin Engine starting...")

	cfg := server.ConfigFromEnv()
	paramStore := params.NewStore()
	idGen := &seqIDGen{}
	callDeadline := 1 * time.Hour

	// Build composite collateral source from available upstream services.
	// Each source is best-effort: if unreachable, it contributes zero.
	compositeSrc := collateral.NewCompositeCollateralSource()

	if addr := os.Getenv("CLEARING_ENGINE_ADDR"); addr != "" {
		compositeSrc.Add("clearing-positions", collateral.NewHTTPCollateralSource(addr))
		log.Printf("Collateral source: clearing-engine positions at %s", addr)
	}

	if addr := os.Getenv("WAREHOUSE_SERVICE_ADDR"); addr != "" {
		// Price provider uses the param store's spot prices for commodity valuation.
		priceProvider := func(commodityID string) (types.Decimal, bool) {
			price, ok := paramStore.GetSpotPrice(commodityID)
			return price, ok
		}
		warehouseOpts := []collateral.WarehouseOption{}
		if h := os.Getenv("WAREHOUSE_HAIRCUT"); h != "" {
			if hv, err := strconv.ParseFloat(h, 64); err == nil {
				warehouseOpts = append(warehouseOpts, collateral.WithHaircut(hv))
				log.Printf("Collateral source: warehouse receipts haircut=%.0f%%", hv*100)
			}
		}
		compositeSrc.Add("warehouse-receipts",
			collateral.NewWarehouseCollateralSource(addr, priceProvider, warehouseOpts...))
		log.Printf("Collateral source: warehouse receipts at %s", addr)
	}

	var colSrc engine.CollateralSource
	if compositeSrc.SourceCount() > 0 {
		colSrc = compositeSrc
		log.Printf("Using composite collateral source (%d sources)", compositeSrc.SourceCount())
	} else {
		colSrc = &inMemoryCollateral{}
		log.Println("Using in-memory collateral source (set CLEARING_ENGINE_ADDR / WAREHOUSE_SERVICE_ADDR for real collateral)")
	}

	eng := engine.NewEngine(paramStore, idGen, colSrc, callDeadline)

	// Set up PostgreSQL persistence when DB_HOST is set
	if dbHost := os.Getenv("DB_HOST"); dbHost != "" {
		dbPort := 5432
		if v := os.Getenv("DB_PORT"); v != "" {
			if p, err := strconv.Atoi(v); err == nil {
				dbPort = p
			}
		}
		dbUser := envOrDefault("DB_USER", "margin")
		dbPass := envOrDefault("DB_PASSWORD", "")
		dbName := envOrDefault("DB_NAME", "garudax")
		dbSSL := envOrDefault("DB_SSLMODE", "disable")

		db, err := store.OpenDB(dbHost, dbPort, dbUser, dbPass, dbName, dbSSL)
		if err != nil {
			log.Fatalf("Failed to connect to PostgreSQL: %v", err)
		}
		defer db.Close()
		log.Printf("Using PostgreSQL store at %s:%d/%s", dbHost, dbPort, dbName)

		portfolioStore := store.NewPostgresPortfolioStore(db)
		callStore := store.NewPostgresMarginCallStore(db)

		// Wire up handlers to persist margin calculations and margin calls to PostgreSQL
		eng.SetMarginHandler(func(pm types.PortfolioMargin) {
			if err := portfolioStore.SavePortfolioMargin(pm); err != nil {
				log.Printf("WARN: failed to persist portfolio margin: %v", err)
			}
			log.Printf("MARGIN: participant=%s required=%s collateral=%s excess_deficit=%s",
				pm.ParticipantID, pm.TotalRequired.String(),
				pm.CollateralOnHand.String(), pm.ExcessDeficit.String())
		})

		eng.SetMarginCallHandler(func(call types.MarginCall) {
			if err := callStore.SaveMarginCall(call); err != nil {
				log.Printf("WARN: failed to persist margin call: %v", err)
			}
			log.Printf("MARGIN CALL: participant=%s deficit=%s deadline=%s",
				call.ParticipantID, call.Deficit.String(), call.Deadline.Format(time.RFC3339))
		})
	} else {
		// In-memory mode: just log
		eng.SetMarginHandler(func(pm types.PortfolioMargin) {
			log.Printf("MARGIN: participant=%s required=%s collateral=%s excess_deficit=%s",
				pm.ParticipantID, pm.TotalRequired.String(),
				pm.CollateralOnHand.String(), pm.ExcessDeficit.String())
		})

		eng.SetMarginCallHandler(func(call types.MarginCall) {
			log.Printf("MARGIN CALL: participant=%s deficit=%s deadline=%s",
				call.ParticipantID, call.Deficit.String(), call.Deadline.Format(time.RFC3339))
		})
	}

	srv := server.NewServer(eng, cfg)

	go func() {
		if err := srv.StartHealthServer(); err != nil {
			log.Fatalf("Health server error: %v", err)
		}
	}()

	lis, err := srv.ListenGRPC()
	if err != nil {
		log.Fatalf("Failed to create gRPC listener: %v", err)
	}

	srv.SetReady()
	log.Printf("GarudaX Margin Engine ready (gRPC=%s, health=%s:%d, direct_pod_comms=%v)",
		lis.Addr().String(), cfg.BindAddress, cfg.HealthPort, cfg.DirectPodComms)

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	sig := <-sigCh
	log.Printf("Received signal %s, shutting down...", sig)
	lis.Close()
}

func envOrDefault(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
