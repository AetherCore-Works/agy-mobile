package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/Phoenix7290/agy-mobile-gateway/packages/cockpit/pkg/cockpit"
)

func envOr(name, fallback string) string {
	if v := strings.TrimSpace(os.Getenv(name)); v != "" {
		return v
	}
	return fallback
}

func envBool(name string, fallback bool) bool {
	v := strings.ToLower(strings.TrimSpace(os.Getenv(name)))
	if v == "" {
		return fallback
	}
	return v == "1" || v == "true" || v == "yes" || v == "on"
}

func envInt(name string, fallback int) int {
	v := strings.TrimSpace(os.Getenv(name))
	if v == "" {
		return fallback
	}
	n, err := strconv.Atoi(v)
	if err != nil || n <= 0 {
		return fallback
	}
	return n
}

func main() {
	addr := envOr("COCKPIT_ADDR", "127.0.0.1:8099")
	sampleSec := envInt("COCKPIT_SAMPLE_INTERVAL_SEC", 5)
	historyMax := envInt("COCKPIT_HISTORY_SAMPLES", 72)
	allowPower := envBool("COCKPIT_ALLOW_POWER", true)
	authToken := strings.TrimSpace(os.Getenv("COCKPIT_TOKEN"))

	col := cockpit.NewSystemCollector()
	store := cockpit.NewSamplerStore(col, time.Duration(sampleSec)*time.Second, historyMax, allowPower)
	pwr := &cockpit.OSPowerController{Delay: 500 * time.Millisecond}

	bgCtx, bgCancel := context.WithCancel(context.Background())
	defer bgCancel()
	store.Start(bgCtx)

	srv := &http.Server{
		Addr: addr,
		Handler: cockpit.NewHandler(cockpit.ServerConfig{
			AllowPower: allowPower,
			AuthToken:  authToken,
		}, store, col, pwr),
		ReadHeaderTimeout: 10 * time.Second,
	}

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)

	go func() {
		log.Printf("AGY Cockpit optional package listening on http://%s (interval=%ds samples=%d allow_power=%v)", addr, sampleSec, historyMax, allowPower)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("cockpit server error: %v", err)
		}
	}()

	<-stop
	bgCancel()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = srv.Shutdown(ctx)
	log.Println("AGY Cockpit stopped cleanly")
}
