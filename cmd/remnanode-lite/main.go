package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"runtime/debug"
	"syscall"
	"time"

	"github.com/12Jack21/remnaplus-alpine-node/internal/asn"
	"github.com/12Jack21/remnaplus-alpine-node/internal/auth"
	"github.com/12Jack21/remnaplus-alpine-node/internal/bodylimit"
	"github.com/12Jack21/remnaplus-alpine-node/internal/config"
	"github.com/12Jack21/remnaplus-alpine-node/internal/connections"
	"github.com/12Jack21/remnaplus-alpine-node/internal/doctor"
	"github.com/12Jack21/remnaplus-alpine-node/internal/httpserver"
	"github.com/12Jack21/remnaplus-alpine-node/internal/netadmin"
	"github.com/12Jack21/remnaplus-alpine-node/internal/plugin"
	"github.com/12Jack21/remnaplus-alpine-node/internal/secret"
	"github.com/12Jack21/remnaplus-alpine-node/internal/system"
	"github.com/12Jack21/remnaplus-alpine-node/internal/unixconfig"
	"github.com/12Jack21/remnaplus-alpine-node/internal/version"
	"github.com/12Jack21/remnaplus-alpine-node/internal/xray"
)

func main() {
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "version", "-version", "--version":
			fmt.Println(version.String())
			return
		case "doctor":
			os.Exit(doctor.Run(os.Args[2:]))
		case "release-url":
			if len(os.Args) < 4 {
				fmt.Fprintf(os.Stderr, "usage: remnanode-lite release-url <tag> <arch>\n")
				os.Exit(2)
			}
			fmt.Println(version.ReleaseAssetURL(os.Args[2], os.Args[3]))
			return
		case "install-script-url":
			if len(os.Args) < 4 {
				fmt.Fprintf(os.Stderr, "usage: remnanode-lite install-script-url <tag> <script>\n")
				os.Exit(2)
			}
			fmt.Println(version.InstallScriptURL(os.Args[2], os.Args[3]))
			return
		}
	}
	cfg, err := config.Load(config.ResolveEnvPath())
	if err != nil {
		log.Fatalf("load config: %v", err)
	}
	applyMemoryLimit(cfg.LowMemory)
	bodylimit.Configure(cfg.LowMemory, cfg.BodyLimitMB)
	if !netadmin.HasCapNetAdmin() {
		log.Printf("warning: CAP_NET_ADMIN not available — nftables plugin and ss -K connection drop are disabled (check setcap on the OpenRC binary)")
	}

	payload, err := secret.Parse(cfg.SecretKey)
	if err != nil {
		log.Fatalf("parse SECRET_KEY: %v", err)
	}

	validator, err := auth.NewJWTValidator(payload.JWTPublicKey)
	if err != nil {
		log.Fatalf("initialize JWT validator: %v", err)
	}

	manager, err := xray.NewManager(xray.Options{
		XrayBin:            cfg.XrayBin,
		GeoDir:             cfg.GeoDir,
		LogDir:             cfg.LogDir,
		DataDir:            cfg.DataDir,
		InternalSocketPath: cfg.InternalSocketPath,
		InternalRESTToken:  cfg.InternalRESTToken,
		DisableHashCheck:   cfg.DisableHashedSetCheck,
		LowMemory:          cfg.LowMemory,
	})
	if err != nil {
		log.Fatalf("initialize Xray manager: %v", err)
	}

	pluginState := plugin.NewState()
	if asnDB, err := asn.Open(cfg.ASNDBPath); err != nil {
		log.Printf("ASN database unavailable (%s): %v — asList shared lists resolve empty", cfg.ASNDBPath, err)
	} else {
		pluginState.SetASNResolver(asnDB)
		defer asnDB.Close()
		log.Printf("ASN database loaded from %s", cfg.ASNDBPath)
	}
	dropper := connections.NewDropper(pluginState.IsWhitelisted)
	pluginService := plugin.NewService(pluginState, dropper, manager)

	manager.SetTorrentBlockerProvider(pluginState)

	server, err := httpserver.New(cfg, payload, validator, manager, pluginService, dropper)
	if err != nil {
		log.Fatalf("initialize HTTPS server: %v", err)
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	unixServer := &unixconfig.Server{
		Path:     cfg.InternalSocketPath,
		Token:    cfg.InternalRESTToken,
		Provider: manager,
		Webhook:  pluginService,
	}
	go func() {
		log.Printf("internal config socket listening on %s", cfg.InternalSocketPath)
		if err := unixServer.ListenAndServe(ctx); err != nil {
			log.Fatalf("internal config socket stopped: %v", err)
		}
	}()

	go func() {
		log.Printf("github.com/12Jack21/remnaplus-alpine-node listening on %s", cfg.HTTPAddr())
		if err := server.ListenAndServeTLS(); err != nil {
			log.Fatalf("HTTPS server stopped: %v", err)
		}
	}()

	go manager.RestoreOnBoot(ctx)
	go manager.StartLogRotation(ctx)

	<-ctx.Done()
	system.DefaultNetworkMonitor().Stop()
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := server.Shutdown(shutdownCtx); err != nil {
		log.Printf("shutdown error: %v", err)
	}
	_ = manager.Stop(false)
}

// applyMemoryLimit caps the Go runtime heap in low-memory mode (128/256MB VPS)
// regardless of init system, replacing launcher-level GOMEMLIMIT settings.
// An explicit GOMEMLIMIT env always
// wins so large nodes are never accidentally throttled.
func applyMemoryLimit(lowMemory bool) {
	if os.Getenv("GOMEMLIMIT") != "" {
		return
	}
	if lowMemory {
		debug.SetMemoryLimit(180 << 20)
		log.Printf("low-memory mode: Go soft memory limit set to 180MiB (override with GOMEMLIMIT)")
	}
}
