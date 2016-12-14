// Provides router with all goroutines
package statsdrouter

import (
	"fmt"
	"log"
	"net"
	"strconv"
	"strings"
	"sync"
	"time"
)

// TODO: Convert these consts to some kind of parameters
const (
	ChannelSize = 4
	WorkerCount = 4
)

// Debug?
var DebugMode bool

// Counter for packets
var Count float32 = 0

// StatsD Metric struct
type StatsDMetric struct {
	name  string
	value float64
	raw   []byte
}

// Starts a new router
// returns an error
func StartRouter(bindAddress string, port uint16, apiPort uint16, masterHost StatsdNode, configPath string, checkInterval int64, quit chan bool) error {
	config, err := NewConfig(configPath)
	if err != nil {
		log.Printf("Error parsing config file: %s (exiting...)", err)
		return err
	}

	masterBackend, err := NewStatsDBackend(masterHost.Host, masterHost.Port, masterHost.ManagementPort, checkInterval)
	if err != nil {
		log.Printf("Failed to create master backend: %s", err)
		return err
	}
	routingMap := NewRoutingMap(checkInterval)
	err = routingMap.UpdateRoutingMap(config)
	if err != nil {
		log.Printf("Failed to populate routing map: %s", err)
		return err
	}

	api := NewHttpApi(apiPort, config, routingMap)
	go api.Start()
	var wg sync.WaitGroup
	go StartMainListener(bindAddress, port, routingMap, masterBackend, quit, &wg)
	// TODO: Add some internal metrics sender goroutine

	// wait for quit signal
	<-quit
	log.Println("Shuting down all backends objects...")
	wg.Add(1)
	go masterBackend.Exit(&wg)
	for _, backend := range routingMap.backendList {
		wg.Add(1)
		go backend.Exit(&wg)
	}
	wg.Wait()
	log.Println("Terminating StartRouter goroutine")
	return nil
}

// Sets up the main UDP listener
// which will send recieved packets to packetHandler via channel
func StartMainListener(bindAddress string, port uint16, routingMap *RoutingMap, masterBackend *StatsDBackend, quit chan bool, wg *sync.WaitGroup) error {
	// TODO: Add quit?
	log.Printf("Starting StatsD listener on %s and port %d", bindAddress, port)

	addr, err := net.ResolveUDPAddr("udp", fmt.Sprintf("%s:%d", bindAddress, port))
	if err != nil {
		log.Printf("Error resolving UDP address [%s:%d]: %s (exiting...)", bindAddress, port, err)
		return err
	}

	conn, err := net.ListenUDP("udp", addr)
	if err != nil {
		log.Printf("Error setting up listener: %s (exiting...)", err)
		return err
	}
	defer conn.Close()

	packetsChannel := make(chan []byte, ChannelSize)
	metricsChannel := make(chan *StatsDMetric, ChannelSize)
	for i := 0; i < WorkerCount; i++ {
		wg.Add(1)
		go packetHandler(packetsChannel, metricsChannel, quit, wg)
	}

	for i := 0; i < WorkerCount; i++ {
		wg.Add(1)
		go metricHandler(routingMap, metricsChannel, masterBackend, quit, wg)
	}
	timeout := float32(10.0)
	tick := time.Tick(time.Duration(timeout) * time.Second)
	padding := strings.Repeat("-", 5)
	if DebugMode {
		go func() {
			for _ = range tick {
				fmt.Printf("%[2]s We got %[1]f packets - %[3]f packets/sec %[2]s\n", Count, padding, Count/timeout)
				Count = 0
			}
		}()
	}
	for {
		buf := make([]byte, 1024)
		packetLength, clientAddr, err := conn.ReadFromUDP(buf)
		if err != nil {
			log.Printf("read err: %s", err)
			continue
		}
		if DebugMode {
			log.Printf("received data from=%s len=%d", clientAddr, packetLength)
			Count++
		}
		packetsChannel <- buf[0:packetLength]
	}
	log.Println("Terminating StartMainListener goroutine")
	return nil
}

// Handles packets, creates metrics from them and sends them to metricHandler via channel
func packetHandler(packetsChannel chan []byte, metricsChannel chan *StatsDMetric, quit chan bool, wg *sync.WaitGroup) {
	defer wg.Done()
	for {
		select {
		case packet := <-packetsChannel:
			if DebugMode {
				log.Printf("Got packet: %s", string(packet))
			}
			metrics := strings.Split(string(packet), "\n")
			for _, metric := range metrics {
				metric := parseMetric(metric)
				metricsChannel <- metric
			}
		case <-quit:
			log.Println("Terminating packetHandler goroutine")
			return
		}
	}
}

// Parses a string into a statsd packet
// accepts a string of data
// returns a StatsDMetric
func parseMetric(data string) *StatsDMetric {
	metric := new(StatsDMetric)
	metricParts := strings.Split(data, ":")
	if len(metricParts) < 2 {
		log.Printf("Malformatted metric: %s", data)
		return metric
	}
	name := metricParts[0]
	valueParts := strings.Split(metricParts[1], "|")
	value64, _ := strconv.ParseInt(valueParts[0], 10, 0)
	value := float64(value64)
	// check for a samplerate
	typeParts := strings.Split(valueParts[1], "@")
	metricType := typeParts[0]

	switch metricType {
	case "c", "ms", "g":
		metric.name = name
		metric.value = value
		metric.raw = []byte(data)
	default:
		log.Printf("Unknown metrics type: %s", metricType)
	}

	return metric
}

// Sends a metric to one of the active statsd backends
func metricHandler(routingMap *RoutingMap, metricsChannel chan *StatsDMetric, masterBackend *StatsDBackend, quit chan bool, wg *sync.WaitGroup) {
	defer wg.Done()
	for {
		select {
		case metric := <-metricsChannel:
			for _, rule := range routingMap.Map {
				// find out to which backend send a metric
				if rule.Regexp.MatchString(metric.name) {
					for _, backend := range rule.Backends {
						if backend.Status.Alive {
							backend.SendChannel <- metric.raw
						}
					}
				}
			}
			if masterBackend.Status.Alive {
				masterBackend.SendChannel <- metric.raw
			}
		case <-quit:
			log.Println("Terminating metricHandler goroutine")
			return
		}
	}
}
