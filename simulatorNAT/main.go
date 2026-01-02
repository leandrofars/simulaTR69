package main

import (
	"bufio"
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"
)

type NATRouter struct {
	// Network settings
	deviceIP   string
	devicePort string
	acsIP      string
	acsPort    string
	listenPort string
	routerIP   string
	publicIP   string

	// Simulation settings
	latency               time.Duration
	returnLatency         time.Duration
	acsConnectionDelay    time.Duration
	listenConnectionDelay time.Duration
	packetLoss            float64 // 0.0 a 1.0
	bandwidthKbps         int

	// State
	mu                     sync.RWMutex
	activeConns            int
	totalRequests          int
	totalResponses         int
	requestsDropped        int
	totalConnectionTime    time.Duration
	lastConnectionDuration time.Duration
}

// NewNATRouter creates a new instance of the NAT router.
func NewNATRouter(deviceIP, devicePort, acsIP, acsPort, listenPort string) *NATRouter {
	return &NATRouter{
		deviceIP:              deviceIP,
		devicePort:            devicePort,
		acsIP:                 acsIP,
		acsPort:               acsPort,
		listenPort:            listenPort,
		routerIP:              "192.168.1.1",
		publicIP:              "203.0.113.1", // Example public IP
		latency:               0,
		returnLatency:         0,
		acsConnectionDelay:    0,
		listenConnectionDelay: 0,
		packetLoss:            0,
		bandwidthKbps:         0, // No limit
	}
}

// Start initializes the NAT server.
func (nr *NATRouter) Start() error {
	listener, err := net.Listen("tcp", ":"+nr.listenPort)
	if err != nil {
		return err
	}
	defer listener.Close()

	log.Printf("[NAT Router] Started on port %s", nr.listenPort)
	log.Printf("[NAT Router] Awaiting device on %s:%s", nr.deviceIP, nr.devicePort)
	log.Printf("[NAT Router] Awaiting ACS on %s:%s", nr.acsIP, nr.acsPort)
	log.Printf("[NAT Router] Configuration: Latency=%v, Loss=%v%%", nr.latency, nr.packetLoss*100)

	for {
		conn, err := listener.Accept()
		if err != nil {
			log.Printf("[Error] Failed to accept connection: %v", err)
			continue
		}

		// Simulate a delay to accept the connection on the listen port
		if nr.listenConnectionDelay > 0 {
			log.Printf("[Delay] Delaying connection acceptance by %v", nr.listenConnectionDelay)
			time.Sleep(nr.listenConnectionDelay)
		}

		go nr.handleConnection(conn)
	}
}

// handleConnection handles the connection between the device and the ACS.
func (nr *NATRouter) handleConnection(clientConn net.Conn) {
	startTime := time.Now()
	defer clientConn.Close()

	clientAddr := clientConn.RemoteAddr().String()
	log.Printf("[Connection] New connection received from %s", clientAddr)

	nr.mu.Lock()
	nr.activeConns++
	nr.mu.Unlock()

	defer func() {
		duration := time.Since(startTime)
		nr.mu.Lock()
		nr.totalConnectionTime += duration
		nr.lastConnectionDuration = duration
		nr.activeConns--
		nr.mu.Unlock()
	}()

	// Simulate a delay to establish the connection with the ACS
	if nr.acsConnectionDelay > 0 {
		log.Printf("[Delay] Delaying connection to ACS by %v", nr.acsConnectionDelay)
		time.Sleep(nr.acsConnectionDelay)
	}

	// Connect to the ACS
	serverConn, err := net.Dial("tcp", nr.acsIP+":"+nr.acsPort)
	if err != nil {
		log.Printf("[Error] Failed to connect to ACS %s:%s - %v", nr.acsIP, nr.acsPort, err)
		return
	}
	defer serverConn.Close()

	log.Printf("[Routing] %s -> ACS (%s:%s)", clientAddr, nr.acsIP, nr.acsPort)

	// Create channels for synchronization
	done := make(chan struct{})

	// Device -> ACS
	go nr.forwardTraffic(clientConn, serverConn, "Device->ACS", done)

	// ACS -> Device
	go nr.forwardTraffic(serverConn, clientConn, "ACS->Device", done)

	// Wait for one of the goroutines to finish
	<-done
	log.Printf("[Disconnection] Connection closed: %s. Duration: %v", clientAddr, time.Since(startTime))
}

// Roteia tráfego entre conexões com simulação de rede
func (nr *NATRouter) forwardTraffic(src, dst net.Conn, direction string, done chan struct{}) {
	defer func() {
		done <- struct{}{}
	}()

	reader := bufio.NewReader(src)
	buffer := make([]byte, 4096)

	for {
		n, err := reader.Read(buffer)
		if err != nil && err != io.EOF {
			log.Printf("[%s] Error reading: %v", direction, err)
			return
		}

		if n == 0 {
			return
		}

		// Simulate packet loss
		if nr.shouldDropPacket() {
			nr.mu.Lock()
			nr.requestsDropped++
			nr.mu.Unlock()
			log.Printf("[%s] Packet dropped (simulation)", direction)
			continue
		}

		// Simulate latency
		if nr.latency > 0 && strings.Contains(direction, "Device") {
			time.Sleep(nr.latency)
		} else if nr.returnLatency > 0 && strings.Contains(direction, "ACS") {
			time.Sleep(nr.returnLatency)
		}

		// Simulate bandwidth limitation
		if nr.bandwidthKbps > 0 {
			delayMs := time.Duration((n*8)/(nr.bandwidthKbps/1000)) * time.Millisecond
			time.Sleep(delayMs)
		}

		// Send to destination
		_, err = dst.Write(buffer[:n])
		if err != nil {
			log.Printf("[%s] Error writing: %v", direction, err)
			return
		}

		nr.logTraffic(direction, buffer[:n])
	}
}

// shouldDropPacket randomly decides if a packet should be dropped.
func (nr *NATRouter) shouldDropPacket() bool {
	if nr.packetLoss <= 0 {
		return false
	}
	return false // Simplified implementation
}

// Registra informações sobre tráfego
func (nr *NATRouter) logTraffic(direction string, data []byte) {
	nr.mu.Lock()
	defer nr.mu.Unlock()

	if strings.Contains(direction, "Device") {
		nr.totalRequests++
	} else {
		nr.totalResponses++
	}

	// Log message type (HTTP, XML, etc.)
	if len(data) > 0 {
		preview := string(data[:min(len(data), 50)])
		if strings.Contains(preview, "POST") || strings.Contains(preview, "HTTP") {
			log.Printf("[%s] HTTP | Size: %d bytes", direction, len(data))
		} else if strings.Contains(preview, "<?xml") {
			log.Printf("[%s] XML | Size: %d bytes", direction, len(data))
		} else {
			log.Printf("[%s] Data | Size: %d bytes", direction, len(data))
		}
	}
}

// SetLatency sets the latency via CLI.
func (nr *NATRouter) SetLatency(ms int64) {
	nr.latency = time.Duration(ms) * time.Millisecond
	log.Printf("[Config] Latency set to %v", nr.latency)
}

func (nr *NATRouter) SetReturnLatency(ms int64) {
	nr.returnLatency = time.Duration(ms) * time.Millisecond
	log.Printf("[Config] Return latency set to %v", nr.returnLatency)
}

func (nr *NATRouter) SetAcsConnectionDelay(ms int64) {
	nr.acsConnectionDelay = time.Duration(ms) * time.Millisecond
	log.Printf("[Config] ACS connection delay set to %v", nr.acsConnectionDelay)
}

func (nr *NATRouter) SetListenConnectionDelay(ms int64) {
	nr.listenConnectionDelay = time.Duration(ms) * time.Millisecond
	log.Printf("[Config] Listen connection delay set to %v", nr.listenConnectionDelay)
}

func (nr *NATRouter) SetPacketLoss(percentage float64) {
	if percentage < 0 {
		percentage = 0
	}
	if percentage > 1 {
		percentage = 1
	}
	nr.packetLoss = percentage
	log.Printf("[Config] Packet loss set to %.2f%%", nr.packetLoss*100)
}

func (nr *NATRouter) SetBandwidth(kbps int) {
	nr.bandwidthKbps = kbps
	if kbps > 0 {
		log.Printf("[Config] Bandwidth limited to %d Kbps", kbps)
	} else {
		log.Printf("[Config] Unlimited bandwidth")
	}
}

func (nr *NATRouter) SetDeviceIP(ip, port string) {
	nr.deviceIP = ip
	nr.devicePort = port
	log.Printf("[Config] Device IP set to %s:%s", ip, port)
}

func (nr *NATRouter) SetACSIP(ip, port string) {
	nr.acsIP = ip
	nr.acsPort = port
	log.Printf("[Config] ACS IP set to %s:%s", ip, port)
}

// StartControlAPI starts the HTTP server for remote control.
func (nr *NATRouter) StartControlAPI(controlPort string) {
	http.HandleFunc("/config/latency", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			var ms int64
			fmt.Sscanf(r.URL.Query().Get("ms"), "%d", &ms)
			nr.SetLatency(ms)
			fmt.Fprintf(w, "Latency set to %d ms", ms)
		}
	})

	http.HandleFunc("/config/return-latency", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			var ms int64
			fmt.Sscanf(r.URL.Query().Get("ms"), "%d", &ms)
			nr.SetReturnLatency(ms)
			fmt.Fprintf(w, "Return latency set to %d ms", ms)
		}
	})

	http.HandleFunc("/config/acs-connection-delay", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			var ms int64
			fmt.Sscanf(r.URL.Query().Get("ms"), "%d", &ms)
			nr.SetAcsConnectionDelay(ms)
			fmt.Fprintf(w, "ACS connection delay set to %d ms", ms)
		}
	})

	http.HandleFunc("/config/listen-connection-delay", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			var ms int64
			fmt.Sscanf(r.URL.Query().Get("ms"), "%d", &ms)
			nr.SetListenConnectionDelay(ms)
			fmt.Fprintf(w, "Listen connection delay set to %d ms", ms)
		}
	})

	http.HandleFunc("/config/packetloss", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			var loss float64
			fmt.Sscanf(r.URL.Query().Get("percent"), "%f", &loss)
			nr.SetPacketLoss(loss / 100)
			fmt.Fprintf(w, "Packet loss set to %.2f%%", loss)
		}
	})

	http.HandleFunc("/config/bandwidth", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			var kbps int
			fmt.Sscanf(r.URL.Query().Get("kbps"), "%d", &kbps)
			nr.SetBandwidth(kbps)
			fmt.Fprintf(w, "Bandwidth set to %d Kbps", kbps)
		}
	})

	http.HandleFunc("/stats", func(w http.ResponseWriter, r *http.Request) {
		nr.mu.RLock()
		defer nr.mu.RUnlock()
		fmt.Fprintf(w, `
NAT Router Statistics:
- Active connections: %d
- Total requests: %d
- Total responses: %d
- Dropped packets: %d
- Router IP: %s
- Public IP: %s
- Last connection duration: %v
- Total connection time: %v
`, nr.activeConns, nr.totalRequests, nr.totalResponses, nr.requestsDropped, nr.routerIP, nr.publicIP, nr.lastConnectionDuration, nr.totalConnectionTime)
	})

	log.Printf("[API] Control API started at http://localhost:%s", controlPort)
	log.Fatal(http.ListenAndServe(":"+controlPort, nil))
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func main() {
	deviceIP := flag.String("device-ip", "127.0.0.1", "TR-069 simulator IP")
	devicePort := flag.String("device-port", "0", "TR-069 simulator port (0 = do not validate)")
	acsIP := flag.String("acs-ip", "127.0.0.1", "ACS IP")
	acsPort := flag.String("acs-port", "9292", "ACS port")
	listenPort := flag.String("listen-port", "9999", "NAT Router listening port")
	controlPort := flag.String("control-port", "8888", "Control API port")
	latency := flag.Int64("latency-ms", 10, "Latency in ms")
	acsConnectionDelay := flag.Int64("acs-connection-delay-ms", 0, "Delay to connect to ACS in ms")
	listenConnectionDelay := flag.Int64("listen-connection-delay-ms", 0, "Delay to accept connection on listen port in ms")
	returnLatency := flag.Int64("return-latency-ms", 10, "Return latency in ms")
	flag.Parse()

	router := NewNATRouter(*deviceIP, *devicePort, *acsIP, *acsPort, *listenPort)
	router.SetLatency(*latency)

	// Start control API in a goroutine
	go router.StartControlAPI(*controlPort)
	router.SetAcsConnectionDelay(*acsConnectionDelay)
	router.SetListenConnectionDelay(*listenConnectionDelay)
	router.SetReturnLatency(*returnLatency)

	// Start NAT router
	if err := router.Start(); err != nil {
		log.Fatalf("[Fatal Error] %v", err)
	}
}
