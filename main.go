package main

import (
	"embed"
	"flag"
	"fmt"
	"io/fs"
	"net/http"
	"os"
	"time"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"

	"github.com/ngate/internal/api"
	"github.com/ngate/internal/certmanager"
	"github.com/ngate/internal/db"
	"github.com/ngate/internal/nginx"
)

//go:embed ui/static/*
var uiFS embed.FS

func main() {
	port := flag.Int("port", 8080, "Admin UI port")
	dataDir := flag.String("data", "/etc/ngate", "Data directory")
	confDir := flag.String("conf", "/etc/nginx/sites-enabled", "Nginx config directory")
	certDir := flag.String("certs", "/etc/ngate/certs", "Certificate directory")
	flag.Parse()

	logrus.SetFormatter(&logrus.TextFormatter{FullTimestamp: true})

	// Ensure dirs
	for _, d := range []string{*dataDir, *confDir, *certDir} {
		os.MkdirAll(d, 0755)
	}

	// Init components
	database, err := db.New(*dataDir + "/proxy-manager.db")
	if err != nil {
		logrus.Fatalf("Failed to init database: %v", err)
	}
	defer database.Close()

	nginxMgr := nginx.New(*confDir, *certDir)
	if err := nginxMgr.EnsureDirs(); err != nil {
		logrus.Warnf("Could not create all dirs: %v", err)
	}

	certMgr := certmanager.New(*certDir)
	handler := api.NewHandler(database, nginxMgr, certMgr)

	// Router
	gin.SetMode(gin.ReleaseMode)
	r := gin.New()
	r.Use(gin.Recovery())
	r.Use(cors.New(cors.Config{
		AllowOrigins:     []string{"*"},
		AllowMethods:     []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
		AllowHeaders:     []string{"*"},
		AllowCredentials: true,
		MaxAge:           12 * time.Hour,
	}))

	// API routes
	handler.RegisterRoutes(r.Group("/api"))

	// Serve embedded UI
	uiContent, err := fs.Sub(uiFS, "ui/static")
	if err != nil {
		logrus.Fatalf("Failed to get UI filesystem: %v", err)
	}
	r.NoRoute(gin.WrapH(http.FileServer(http.FS(uiContent))))

	addr := fmt.Sprintf(":%d", *port)
	logrus.Infof("Ngate running on http://0.0.0.0%s", addr)
	logrus.Infof("Data: %s | Nginx conf: %s | Certs: %s", *dataDir, *confDir, *certDir)

	if err := r.Run(addr); err != nil {
		logrus.Fatalf("Server failed: %v", err)
	}
}
