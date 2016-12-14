package main

import (
	"flag"
	"github.com/antonsoroko/statsd-router/statsdrouter"
	"log"
	"os"
	"os/signal"
	"syscall"
)

var (
	configFile       = flag.String("config", "statsd-router.json", "Configuration file path")
	bindAddress      = flag.String("bind-address", "0.0.0.0", "Address to bind")
	port             = flag.Uint("port", 48125, "Port to use")
	apiPort          = flag.Uint("api-port", 48126, "Port to use")
	masterHostString = flag.String("master-statsd-host", "localhost:8125:8126", "Host that will receive all metrics. Format is host:port:mgmt_port")
	checkInterval    = flag.Int64("check-interval", 180, "Interval of checking for backend health")
	debug            = flag.Bool("debug", false, "Enable debug mode")
)

func main() {
	flag.Parse()
	masterHost, err := statsdrouter.NewStatsdNode(*masterHostString)
	if err != nil {
		log.Fatalf("Failed to convert master-statsd-host to StatsdNode: %s", err)
	}
	log.Printf("Using %+v as master host", masterHost)
	statsdrouter.DebugMode = *debug

	quit := make(chan bool)

	handleSignals(quit)

	statsdrouter.StartRouter(
		*bindAddress,
		uint16(*port),
		uint16(*apiPort),
		masterHost,
		*configFile,
		*checkInterval,
		quit,
	)
	log.Println("Exit.")
}

func handleSignals(quit chan bool) {
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, os.Interrupt, syscall.SIGTERM, syscall.SIGHUP)
	go func(chan os.Signal) {
		defer signal.Reset()
		defer signal.Stop(sigs)
	loop:
		for sig := range sigs {
			log.Printf("caught signal: %+v", sig)
			switch sig {
			case os.Interrupt, syscall.SIGTERM:
				log.Println("Sending quit signal to goroutines...")
				close(quit)
				break loop
			case syscall.SIGHUP:
				// TODO: implement real reloading?
				log.Println("Reloading...")
			}
			log.Println("Terminating Signals Handler.")
		}
	}(sigs)
}
