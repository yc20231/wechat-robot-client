package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"business-gateway/internal/backend"
	"business-gateway/internal/config"
	"business-gateway/internal/dedup"
	"business-gateway/internal/group"
	"business-gateway/internal/httpapi"
	"business-gateway/internal/route"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("加载配置失败: %v", err)
	}
	groups, err := group.NewFileStore(cfg.BindingsFile)
	if err != nil {
		log.Fatalf("加载群绑定失败: %v", err)
	}
	backendClient, err := backend.NewClient(cfg.BackendURL, cfg.BotToken, cfg.BackendTimeout)
	if err != nil {
		log.Fatalf("创建后端客户端失败: %v", err)
	}
	deduplicator := dedup.NewMemoryCache(cfg.DedupTTL)
	router := route.NewService(groups, backendClient, deduplicator, cfg.AdminWxIDs, cfg.RequireAtMention)
	handler := httpapi.NewHandler(cfg, groups, router, deduplicator)

	server := &http.Server{
		Addr:              cfg.ListenAddr,
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      20 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	shutdownCtx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	go func() {
		<-shutdownCtx.Done()
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := server.Shutdown(ctx); err != nil {
			log.Printf("关闭 HTTP 服务失败: %v", err)
		}
	}()

	log.Printf("business-gateway 监听 %s，已加载 %d 个群绑定", cfg.ListenAddr, len(groups.List()))
	if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Printf("HTTP 服务异常退出: %v", err)
		os.Exit(1)
	}
}
