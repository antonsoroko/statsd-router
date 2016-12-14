// Backend struct and its methods
package statsdrouter

import (
	"fmt"
	"log"
	"net"
	"os"
	"strings"
	"sync"
	"syscall"
	"time"
)

// StatsD Backend struct
type StatsDBackend struct {
	Host           string
	Port           uint16
	ManagementPort uint16
	conn           net.Conn
	ManagementConn net.Conn
	SendChannel    chan []byte
	Status         struct {
		Alive        bool
		LastPingTime int64
	}
	healthCheckInterval int64
	quit                chan bool
	wg                  sync.WaitGroup
}

func (backend StatsDBackend) String() string {
	return fmt.Sprintf("StatsDBackend{Host:%q, Port:%d, ManagementPort:%d}", backend.Host, backend.Port, backend.ManagementPort)
}

// Creates a new StatsDBackend struct
// accepts a host, port, managementPort and checkInterval as parameters
// returns the StatsDBackend struct and an error
func NewStatsDBackend(host string, port uint16, managementPort uint16, checkInterval int64) (*StatsDBackend, error) {
	backend := StatsDBackend{Host: host, Port: port, ManagementPort: managementPort, healthCheckInterval: checkInterval}
	backend.SendChannel = make(chan []byte, ChannelSize)
	backend.quit = make(chan bool)
	err := backend.Open()
	if err != nil {
		log.Printf("Failed to create %s: %s", backend, err)
		return nil, err
	}
	err = backend.OpenManagementConnection()
	if err != nil {
		log.Printf("Failed to create %s: %s", backend, err)
		return nil, err
	}
	backend.CreateAliveChecker()
	backend.CreateSender()
	return &backend, nil
}

// Opens udp connection
func (backend *StatsDBackend) Open() error {
	addr, err := net.ResolveUDPAddr("udp", fmt.Sprintf("%s:%d", backend.Host, backend.Port))
	if err != nil {
		log.Printf("Error resolving UDP address [%s:%d]: %s", backend.Host, backend.Port, err)
		return err
	}

	conn, err := net.DialUDP("udp", nil, addr)
	if err != nil {
		log.Printf("Error dial to UDP address %v: %s", addr, err)
		return err
	}
	backend.conn = conn
	return nil
}

// Opens TCP connection to the management port
func (backend *StatsDBackend) OpenManagementConnection() error {
	addr, err := net.ResolveTCPAddr("tcp", fmt.Sprintf("%s:%d", backend.Host, backend.ManagementPort))
	if err != nil {
		log.Printf("Error resolving TCP address [%s:%d]: %s", backend.Host, backend.ManagementPort, err)
		return err
	}

	conn, err := net.DialTCP("tcp", nil, addr)
	handled := false
	if err != nil {
		log.Printf("Error dial to TCP address %v: %s", addr, err)
		// TODO: add *net.timeoutError handling
		if opErr, ok := err.(*net.OpError); ok {
			if sysErr, ok := opErr.Err.(*os.SyscallError); ok {
				if sysErr.Err == syscall.ECONNREFUSED {
					handled = true
					log.Println("Got ECONNREFUSED!")
				}
			}
		}
		if !handled {
			return err
		}
	}
	if handled {
		backend.ManagementConn = nil
	} else {
		backend.ManagementConn = conn
	}
	return nil
}

// Shutdown goroutines and close all connections
func (backend *StatsDBackend) Exit(externalWG *sync.WaitGroup) {
	defer externalWG.Done()
	log.Println("Terminating backend:", backend)
	close(backend.quit)
	close(backend.SendChannel)
	backend.wg.Wait()
	if backend.ManagementConn != nil {
		backend.ManagementConn.Close()
	}
	backend.conn.Close()
	log.Println("TERMINATED backend:", backend)
}

// Creates senders
func (backend *StatsDBackend) CreateSender() {
	if DebugMode {
		log.Printf("Creating sender goroutine for %s", backend)
	}
	for i := 0; i < WorkerCount; i++ {
		backend.wg.Add(1)
		go func(index int) {
			defer backend.wg.Done()
			for metric := range backend.SendChannel {
				if DebugMode {
					log.Printf("Sending %s to backend %s", metric, backend)
				}
				if _, err := backend.conn.Write(metric); err != nil {
					log.Println(err)
				}
			}
			if DebugMode {
				log.Printf("Terminating Sender goroutine #%d of backend %s", index, backend)
			}
		}(i)
	}
}

// Creates aliveness checker
// This checker will check backend every healthCheckInterval seconds
func (backend *StatsDBackend) CreateAliveChecker() {
	if DebugMode {
		log.Printf("Creating alive checker goroutine for %s", backend)
	}

	backend.Status.Alive = backend.CheckAliveStatus()
	if !backend.Status.Alive {
		log.Printf("Freshly created backend %s is not alive by the way", backend)
	}

	backend.wg.Add(1)
	go func() {
		defer backend.wg.Done()
		tick := time.Tick(time.Duration(backend.healthCheckInterval) * time.Second)
		for {
			select {
			case <-tick:
				backend.Status.Alive = backend.CheckAliveStatus()
			case <-backend.quit:
				log.Printf("Terminating CreateAliveChecker goroutine for backend %s", backend)
				return
			}
		}
	}()
}

// Checks aliveness of backend
// Function tries to reconnect to management port 'retryCount' times
// returns false or true
func (backend *StatsDBackend) CheckAliveStatus() bool {
	var err error
	var retryCount = 0
	if DebugMode {
		log.Printf("Checking backend %s", backend)
	}
	statusString := []byte("health")
	// TODO: don't make last backend.OpenManagementConnection call if we already exceeded retryCount
Retry:
	if retryCount > 1 {
		log.Printf("Give up after #%d attemps for %s", retryCount, backend)
		return false
	}
	if backend.ManagementConn == nil {
		err = backend.OpenManagementConnection()
		if err != nil || backend.ManagementConn == nil {
			log.Printf("Failed to OpenManagementConnection for %s: %s", backend, err)
			retryCount++
			log.Printf("Retry #%d for %s", retryCount, backend)
			goto Retry
		}
	}
	backend.ManagementConn.SetWriteDeadline(time.Now().Add(2 * time.Second))
	_, err = backend.ManagementConn.Write(statusString)
	if err != nil {
		log.Printf("Write to %s failed: %s", backend, err)
		retryCount++
		log.Printf("Retry #%d for %s", retryCount, backend)
		err = backend.OpenManagementConnection()
		if err != nil {
			log.Printf("Failed to OpenManagementConnection for %s: %s", backend, err)
			return false
		}
		goto Retry
	}
	reply := make([]byte, 1024)

	backend.ManagementConn.SetReadDeadline(time.Now().Add(2 * time.Second))
	_, err = backend.ManagementConn.Read(reply)
	if err != nil {
		log.Printf("Read from %s failed: %s", backend, err)
		retryCount++
		log.Printf("Retry #%d for %s", retryCount, backend)
		err = backend.OpenManagementConnection()
		if err != nil {
			log.Printf("Failed to OpenManagementConnection for %s: %s", backend, err)
			return false
		}
		goto Retry
	}
	healthStatus := strings.Trim(string(reply), string(0))

	if DebugMode {
		log.Printf("Response from backend %s: %s", backend, healthStatus)
	}
	if strings.Contains(healthStatus, "up") {
		if DebugMode {
			log.Printf("Backend %s is up", backend)
		}
		return true
	} else {
		if DebugMode {
			log.Printf("Backend %s is down", backend)
		}
		return false
	}
}
