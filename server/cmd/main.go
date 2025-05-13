package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"sync"
	"syscall"
	"time"

	"github.com/grandcat/zeroconf"
	"github.com/nxtcoder17/nxtpass/server/internal/store"
	"github.com/nxtcoder17/nxtpass/server/internal/store/models"

	"github.com/caarlos0/env/v11"
	"github.com/nxtcoder17/fastlog"
	"github.com/nxtcoder17/ivy"
	"github.com/nxtcoder17/ivy/middleware"
)

const (
	serviceName   = "_nxtpass._http"
	serviceDomain = "local"
)

var envVar struct {
	// Port must be auto allocated as per availability, unless in dev mode
	Port     int    `env:"NXTPASS_PORT" default:"0"`
	Instance string `env:"NXTPASS_INSTANCE" required:"true"`
	DB       string `env:"NXTPASS_DB" default:"sqlite.db"`
}

func sqltest() {
	s, err := store.NewSQLiteStore(envVar.DB)
	if err != nil {
		panic(err)
	}

	r, err := s.Create(context.TODO(), models.Credential{
		Username:  "sample",
		Password:  "sample",
		Hosts:     []string{"sample.com", "sample2.com", "sample3.com"},
		Extra:     map[string]string{"email": "sample@sample.com"},
		Tags:      []string{"sample"},
		Namespace: "sample",
	})
	if err != nil {
		panic(err)
	}

	slog.Info("main", "inserted.record", r)
}

var logger *fastlog.Logger

func registerService(ctx context.Context) {
	slog.Info("Registering zeroconf service", "instance", envVar.Instance, "port", envVar.Port)
	server, err := zeroconf.Register(envVar.Instance, serviceName, serviceDomain, envVar.Port, []string{"nxtpass=true"}, nil)
	if err != nil {
		panic(err)
	}

	<-ctx.Done()
	server.Shutdown()
}

type Peer struct {
	Instance     string
	Addr         string
	LastSyncedAt time.Time
}

func onPeerFound(ctx context.Context, store store.Store, peer *Peer) error {
	logger.Info("found", "peer", peer.Addr)

	localCtx, cf := context.WithTimeout(ctx, 5*time.Second)
	defer cf()

	checkpoint, err := store.LastCheckpointAt(localCtx)
	if err != nil {
		return err
	}

	logger.Debug("store", "peer", peer.Instance, "checkpoint", checkpoint)

	req, err := http.NewRequestWithContext(localCtx, http.MethodGet, "http://"+peer.Addr+"/sync-stream", nil)
	if err != nil {
		return err
	}

	qp := req.URL.Query()
	qp.Add("since", fmt.Sprint(checkpoint))
	req.URL.RawQuery = qp.Encode()

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}

	reader := bufio.NewReader(resp.Body)
	defer resp.Body.Close()

	for {
		b, err := reader.ReadBytes('\n')
		if err != nil {
			if errors.Is(err, io.EOF) {
				return err
			}
		}

		if len(b) == 0 {
			break
		}

		logger.Debug("[received]", "message", string(b), "message.len", len(b))

		var msg models.ActivityLog
		if err := json.Unmarshal(b, &msg); err != nil {
			logger.Error("unmarshal failed", "err", err)
		}

		if err := store.SyncRecord(ctx, msg.Timestamp, msg.SQLQuery); err != nil {
			logger.Error("sync record", "err", err)
			return err
		}
	}

	return nil
}

func watchForPeers(ctx context.Context, store store.Store) {
	peers := sync.Map{}

	for {
		// start := time.Now()
		// logger.Debug("client browsing cycle started")
		resolver, err := zeroconf.NewResolver(nil)
		if err != nil {
			logger.Error("failed to initialize zeroconf resolver", "err", err)
			panic(err)
		}

		entries := make(chan *zeroconf.ServiceEntry)
		go func(results <-chan *zeroconf.ServiceEntry) {
			for entry := range results {
				if entry.Instance == envVar.Instance {
					continue
				}

				hashKey := entry.Instance

				if _, ok := peers.Load(hashKey); !ok {
					peer := &Peer{
						Instance:     entry.Instance,
						Addr:         fmt.Sprintf("%s:%d", entry.HostName, entry.Port),
						LastSyncedAt: time.Now(),
					}
					peers.Store(hashKey, struct{}{})

					go func(key string) {
						if err := onPeerFound(ctx, store, peer); err != nil {
							logger.Error("onPeerFound", "err", err)
						}
						peers.Delete(key)
					}(hashKey)
				}
			}
			// logger.Debug("client browsing cycle finished", "took", fmt.Sprintf("%.2fs", time.Since(start).Seconds()))
		}(entries)

		err = resolver.Browse(ctx, serviceName, serviceDomain, entries)
		if err != nil {
			logger.Error("failed to browse", "err", err)
			panic(err)
		}

		watchCtx, cancel := context.WithTimeout(ctx, time.Second*5)
		defer cancel()

		select {
		case <-ctx.Done():
			return
		case <-watchCtx.Done():
			return
		case <-time.After(5 * time.Second):
		}
	}
}

func httpServer(ctx context.Context, listener net.Listener, store store.Store) {
	router := ivy.NewRouter()

	router.Get("/ping", func(c *ivy.Context) error {
		return c.SendString("hello from " + envVar.Instance)
	})

	router.Post("/cred", func(c *ivy.Context) error {
		var cred models.Credential
		if err := c.ParseBodyInto(&cred); err != nil {
			return errors.Join(fmt.Errorf("failed to parse request body"), err)
		}

		cred.CreatedBy = envVar.Instance
		cred.CreatedAt = time.Now().Unix()
		cred.UpdatedAt = time.Now().Unix()

		id, err := store.Create(c, cred)
		if err != nil {
			return errors.Join(fmt.Errorf("failed to create credential in store"), err)
		}

		return c.JSON(map[string]any{"id": id})
	})

	router.Get("/sync-stream", middleware.RequiredQueryParams("since"), func(c *ivy.Context) error {
		since := c.QueryParam("since")
		sinceInt, err := strconv.ParseInt(since, 10, 64)
		if err != nil {
			return err
		}
		if err := store.ChangeStream(ctx, sinceInt, c); err != nil {
			logger.Error("failed calling store.changeStream", "err", err)
			return err
		}
		return nil
	})

	logger.Info("starting http server", "addr", fmt.Sprintf(":%d", envVar.Port))
	if err := http.Serve(listener, router); err != nil {
		panic(err)
	}
}

func main() {
	ctx, cf := signal.NotifyContext(context.TODO(), syscall.SIGINT, syscall.SIGTERM)
	defer cf()

	logger = fastlog.New(fastlog.Options{
		Format:        fastlog.ConsoleFormat,
		ShowDebugLogs: os.Getenv("DEBUG") == "true",
		ShowCaller:    true,
		EnableColors:  true,
	})
	slog.SetDefault(logger.Slog())

	if err := env.Parse(&envVar); err != nil {
		logger.Error("failed to parse env variables", "err", err)
		panic(err)
	}

	listener, err := net.Listen("tcp", fmt.Sprintf(":%d", envVar.Port))
	if err != nil {
		panic(err)
	}
	defer listener.Close()

	envVar.Port = listener.Addr().(*net.TCPAddr).Port

	store, err := store.NewSQLiteStore(envVar.DB)
	if err != nil {
		panic(err)
	}

	var wg sync.WaitGroup

	wg.Add(1)
	go func() {
		defer wg.Done()
		registerService(ctx)
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		watchForPeers(ctx, store)
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		httpServer(ctx, listener, store)
	}()

	wg.Wait()
	logger.Info("Shutting down.")
}
