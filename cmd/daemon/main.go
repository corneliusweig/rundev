package main

import (
	"context"
	"encoding/json"
	"flag"
	"github.com/ahmetb/rundev/lib/ignore"
	"github.com/ramr/go-reaper"
	"log"
	"net/http"
	"os"
	"os/signal"
	"time"
)

var (
	flRunCmd               string
	flBuildCmds            string
	flAddr                 string
	flSyncDir              string
	flClientSecret         string
	flIgnorePatterns       string
	flChildPort            int
	flProcessListenTimeout time.Duration
)

func init() {
	log.SetFlags(log.Lmicroseconds)
	listenAddr := "localhost:8080"
	if p := os.Getenv("PORT"); p != "" {
		listenAddr = ":" + p
	}
	flag.StringVar(&flSyncDir, "sync-dir", ".", "directory to sync")
	flag.StringVar(&flClientSecret, "client-secret", "", "(optional) secret to authenticate patches from rundev client")
	flag.StringVar(&flAddr, "addr", listenAddr, "network address to start the daemon")
	flag.StringVar(&flBuildCmds, "build-cmds", "", "(JSON encoded [][]string) commands to rebuild the user app (inside the container)")
	flag.StringVar(&flRunCmd, "run-cmd", "", "(JSON array encoded as string) command to start the user app (inside the container)")
	flag.StringVar(&flIgnorePatterns, "ignore-patterns", "", "(JSON array encoded as string) exclusion rules in .dockerignore")
	flag.IntVar(&flChildPort, "user-port", 5555, "PORT environment variable passed to the user app")
	flag.DurationVar(&flProcessListenTimeout, "process-listen-timeout", time.Second*4, "time to wait for user app to listen on PORT")
	flag.Parse()
}

func main() {
	log.Printf("rundevd running as pid %d", os.Getpid())
	// TODO(ahmetb) instead of crashing the process on flag errors, consider serving error response type so it encourages a redeploy
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go reaper.Reap() // activates when PID 1
	signalCh := make(chan os.Signal, 1)
	signal.Notify(signalCh, os.Interrupt)
	go func() {
		sig := <-signalCh
		log.Printf("[debug] termination signal received: %s", sig)
		cancel()
	}()

	if flSyncDir == "" {
		log.Fatal("-sync-dir is empty")
	}
	// TODO(ahmetb) check if flSyncDir is a directory
	if flAddr == "" {
		log.Fatal("-addr is empty")
	}
	if flProcessListenTimeout <= 0 {
		log.Fatal("-process-listen-timeout must be positive")
	}
	if flChildPort <= 0 || flChildPort > 65535 {
		log.Fatalf("-user-port value (%d) is invalid", flChildPort)
	}
	if flRunCmd == "" {
		log.Fatal("-run-cmd is empty")
	}
	var runCmds []string
	if err := json.Unmarshal([]byte(flRunCmd), &runCmds); err != nil {
		log.Fatalf("failed to parse -run-cmd: %v", err)
	} else if len(runCmds) == 0 {
		log.Fatal("-run-cmd parsed into zero tokens")
	}
	runCmd := &cmd{runCmds[0], runCmds[1:]}

	var buildCmds []cmd
	if flBuildCmds != "" {
		var bcs [][]string
		if err := json.Unmarshal([]byte(flBuildCmds), &bcs); err != nil {
			log.Fatalf("failed to parse -build-cmds: %s", err)
		}
		for _, v := range bcs {
			buildCmds = append(buildCmds, cmd{v[0], v[1:]})
		}
	}

	var ignorePatterns []string
	if flIgnorePatterns != "" {
		if err := json.Unmarshal([]byte(flIgnorePatterns), &ignorePatterns); err != nil {
			log.Fatalf("failed to parse -ignore-patterns: %v", err)
		}
	}


	handler := newDaemonServer(daemonOpts{
		clientSecret:    flClientSecret,
		syncDir:         flSyncDir,
		runCmd:          runCmd,
		buildCmds:       buildCmds,
		childPort:       flChildPort,
		portWaitTimeout: flProcessListenTimeout,
		ignores:         ignore.NewFileIgnores(ignorePatterns),
	})

	localServer := http.Server{
		Handler: handler,
		Addr:    flAddr}
	go func() {
		<-ctx.Done()
		log.Println("shutting down daemon server")
		localServer.Shutdown(ctx)
	}()
	log.Printf("daemon server starting at %s", flAddr)
	if err := localServer.ListenAndServe(); err != nil {
		if err == http.ErrServerClosed {
			log.Printf("local server shut down gracefully, exiting")
			os.Exit(0)
		}
		log.Fatalf("local server failed to start: %+v", err)
	}
}
