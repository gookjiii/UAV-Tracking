package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/grpc-ecosystem/grpc-gateway/v2/runtime"
	"github.com/uav_tracking/api/proto/drone"
	"github.com/uav_tracking/internal/config"
	"github.com/uav_tracking/internal/memory"
	"github.com/uav_tracking/internal/pubsub"
	"github.com/uav_tracking/internal/repository"
	"github.com/uav_tracking/internal/service"
	"github.com/uav_tracking/internal/simulator"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/protobuf/encoding/protojson"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("Invalid configuration: %v", err)
	}
	log.Printf("Starting UAV Real-Time Tracking & Simulation System")
	log.Printf("Target Drones: %d | Update Interval: %d ms | gRPC: %s | HTTP: %s",
		cfg.TargetDroneCount, cfg.UpdateIntervalMS, cfg.GRPCPort, cfg.HTTPPort)

	// 1. In-memory Sharded Cache
	cache := memory.NewMemoryCache(cfg.MemoryHistoryPoints)

	// 2. NATS PubSub Service
	natsSvc, err := pubsub.NewNATSService(cfg.NATSURL, cfg.NATSMaxBytes)
	if err != nil {
		log.Printf("Warning: NATS connection offline (%v). Operating in local memory pubsub mode.", err)
	} else {
		defer natsSvc.Close()
	}

	// 3. PostgreSQL Partitioned Repository
	repo, err := repository.NewPostgresRepository(cfg.PostgresDSN, cfg.BatchSize, cfg.RetentionDays)
	if err != nil {
		log.Printf("Warning: PostgreSQL offline (%v). Operating without persistent database history.", err)
	} else {
		defer repo.Close()
	}

	// 4. Single-write telemetry pipeline and simulation engine.
	pipeline := service.NewTelemetryPipeline(cache, repo, natsSvc, cfg.HistorySampleInterval)
	defer pipeline.Close()
	simEngine := simulator.NewSimulationEngine(
		cfg.TargetDroneCount,
		cfg.UpdateIntervalMS,
		pipeline,
	)
	simEngine.Start()
	defer simEngine.Stop()

	// 5. gRPC Server
	lis, err := net.Listen("tcp", cfg.GRPCPort)
	if err != nil {
		log.Fatalf("Failed to listen on gRPC port %s: %v", cfg.GRPCPort, err)
	}

	grpcServer := grpc.NewServer()
	droneSrv := service.NewDroneServer(cache, repo, simEngine)
	dronepb.RegisterDroneServiceServer(grpcServer, droneSrv)

	go func() {
		log.Printf("gRPC Server listening at %s", cfg.GRPCPort)
		if err := grpcServer.Serve(lis); err != nil {
			log.Printf("gRPC server error: %v", err)
		}
	}()

	// 6. HTTP Server (gRPC-Gateway + SSE Stream Bridge + Static Web Dashboard)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	gwMux := runtime.NewServeMux(
		runtime.WithMarshalerOption(runtime.MIMEWildcard, &runtime.JSONPb{
			MarshalOptions:   protojson.MarshalOptions{UseProtoNames: true, EmitUnpopulated: true},
			UnmarshalOptions: protojson.UnmarshalOptions{DiscardUnknown: true},
		}),
	)

	opts := []grpc.DialOption{grpc.WithTransportCredentials(insecure.NewCredentials())}
	grpcEndpoint := cfg.GRPCPort
	if strings.HasPrefix(grpcEndpoint, ":") {
		grpcEndpoint = "localhost" + grpcEndpoint
	}

	if err := dronepb.RegisterDroneServiceHandlerFromEndpoint(ctx, gwMux, grpcEndpoint, opts); err != nil {
		log.Printf("Warning: gRPC gateway registration issue: %v", err)
	}

	// Stream Bridge for Web SSE
	streamBridge := service.NewStreamBridge(cache, cfg.SSEInterval)
	defer streamBridge.Close()

	// Custom HTTP Router
	httpMux := http.NewServeMux()

	// Serve SSE Stream
	httpMux.HandleFunc("/v1/drones/stream", streamBridge.ServeSSE)
	httpMux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), time.Second)
		defer cancel()
		lastUpdate := cache.LastUpdated()
		ageMS := int64(-1)
		if !lastUpdate.IsZero() {
			ageMS = time.Since(lastUpdate).Milliseconds()
		}
		response := map[string]any{
			"status":                 "ok",
			"simulation_active":      simEngine.IsActive(),
			"target_drones":          simEngine.TargetCount(),
			"current_drones":         cache.Count(),
			"snapshot_age_ms":        ageMS,
			"nats_connected":         natsSvc != nil && natsSvc.Healthy(),
			"postgres_connected":     repo != nil && repo.Healthy(ctx),
			"dropped_nats_snapshots": pipeline.DroppedNATSSnapshots(),
			"dropped_db_points":      pipeline.DroppedDBPoints(),
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(response)
	})

	// Serve Swagger JSON
	httpMux.HandleFunc("/swagger/drone.swagger.json", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		http.ServeFile(w, r, "api/swagger/drone/drone.swagger.json")
	})

	// Serve Swagger UI
	httpMux.HandleFunc("/swagger/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/swagger/" || r.URL.Path == "/swagger/index.html" {
			serveSwaggerUI(w, r)
			return
		}
		http.NotFound(w, r)
	})

	// Serve Flutter Web Dashboard Static Files
	webDir, _ := filepath.Abs(filepath.Join("app", "build", "web"))
	if _, err := os.Stat(webDir); err != nil {
		log.Printf("Warning: Flutter Web build directory (%s) not found. Run 'flutter build web' in app directory.", webDir)
	} else {
		log.Printf("Serving Flutter Web Frontend from %s", webDir)
	}

	fileServer := http.FileServer(http.Dir(webDir))
	httpMux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/v1/") {
			gwMux.ServeHTTP(w, r)
			return
		}
		setStaticCacheHeaders(w, r.URL.Path)
		fileServer.ServeHTTP(w, r)
	})

	httpServer := &http.Server{
		Addr:              cfg.HTTPPort,
		Handler:           withCORS(httpMux),
		ReadHeaderTimeout: 5 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	go func() {
		log.Printf("HTTP Web Dashboard & REST Gateway listening at http://localhost%s", cfg.HTTPPort)
		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Printf("HTTP server error: %v", err)
		}
	}()

	// Graceful Shutdown Handler
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	<-stop

	log.Println("Shutting down servers gracefully...")
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer shutdownCancel()
	grpcStopped := make(chan struct{})
	go func() {
		grpcServer.GracefulStop()
		close(grpcStopped)
	}()
	select {
	case <-grpcStopped:
	case <-shutdownCtx.Done():
		grpcServer.Stop()
	}
	if err := httpServer.Shutdown(shutdownCtx); err != nil {
		log.Printf("HTTP shutdown did not complete cleanly: %v", err)
	}
	log.Println("UAV Tracking Server stopped.")
}

// withCORS keeps the research server usable from a separately served Flutter
// web dev client. Native clients do not need these headers, but browsers need
// them for streamed SSE and JSON POST preflight requests.
func withCORS(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		if origin == "" {
			origin = "*"
		} else {
			w.Header().Set("Vary", "Origin")
		}
		w.Header().Set("Access-Control-Allow-Origin", origin)
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Accept, Cache-Control, Content-Type, ngrok-skip-browser-warning")
		w.Header().Set("Access-Control-Expose-Headers", "Content-Type")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func setStaticCacheHeaders(w http.ResponseWriter, path string) {
	switch {
	case path == "/", strings.HasSuffix(path, "/index.html"),
		strings.HasSuffix(path, "/main.dart.js"),
		strings.HasSuffix(path, "/flutter_bootstrap.js"),
		strings.HasSuffix(path, "/flutter_service_worker.js"):
		w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
	default:
		w.Header().Set("Cache-Control", "public, max-age=604800")
	}
}

func serveSwaggerUI(w http.ResponseWriter, r *http.Request) {
	html := `<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="UTF-8">
  <title>Drone Tracking API - Swagger UI</title>
  <link rel="stylesheet" href="https://unpkg.com/swagger-ui-dist@5/swagger-ui.css" />
</head>
<body>
  <div id="swagger-ui"></div>
  <script src="https://unpkg.com/swagger-ui-dist@5/swagger-ui-bundle.js"></script>
  <script>
    window.onload = () => {
      window.ui = SwaggerUIBundle({
        url: '/swagger/drone.swagger.json',
        dom_id: '#swagger-ui',
      });
    };
  </script>
</body>
</html>`
	w.Header().Set("Content-Type", "text/html")
	fmt.Fprint(w, html)
}
