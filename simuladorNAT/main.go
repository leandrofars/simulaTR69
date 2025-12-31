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
	// Configurações de rede
	deviceIP   string
	devicePort string
	acsIP      string
	acsPort    string
	listenPort string
	routerIP   string
	publicIP   string

	// Configurações de simulação
	latency            time.Duration
	returnLatency      time.Duration
	acsConnectionDelay time.Duration
	packetLoss         float64 // 0.0 a 1.0
	bandwidthKbps      int

	// Estado
	mu                     sync.RWMutex
	activeConns            int
	totalRequests          int
	totalResponses         int
	requestsDropped        int
	totalConnectionTime    time.Duration
	lastConnectionDuration time.Duration
}

// Nova instância do router NAT
func NewNATRouter(deviceIP, devicePort, acsIP, acsPort, listenPort string) *NATRouter {
	return &NATRouter{
		deviceIP:           deviceIP,
		devicePort:         devicePort,
		acsIP:              acsIP,
		acsPort:            acsPort,
		listenPort:         listenPort,
		routerIP:           "192.168.1.1",
		publicIP:           "203.0.113.1", // Exemplo de IP público
		latency:            0,
		returnLatency:      0,
		acsConnectionDelay: 0,
		packetLoss:         0,
		bandwidthKbps:      0, // Sem limitação
	}
}

// Inicia o servidor NAT
func (nr *NATRouter) Start() error {
	listener, err := net.Listen("tcp", ":"+nr.listenPort)
	if err != nil {
		return err
	}
	defer listener.Close()

	log.Printf("[NAT Router] Iniciado em porta %s", nr.listenPort)
	log.Printf("[NAT Router] Device esperado em %s:%s", nr.deviceIP, nr.devicePort)
	log.Printf("[NAT Router] ACS esperado em %s:%s", nr.acsIP, nr.acsPort)
	log.Printf("[NAT Router] Configuração: Latência=%v, Perda=%v%%", nr.latency, nr.packetLoss*100)

	for {
		conn, err := listener.Accept()
		if err != nil {
			log.Printf("[Erro] Falha ao aceitar conexão: %v", err)
			continue
		}

		go nr.handleConnection(conn)
	}
}

// Manipula conexão entre device e ACS
func (nr *NATRouter) handleConnection(clientConn net.Conn) {
	startTime := time.Now()
	defer clientConn.Close()

	clientAddr := clientConn.RemoteAddr().String()
	log.Printf("[Conexão] Nova conexão recebida de %s", clientAddr)

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

	// Simula um atraso para estabelecer a conexão com o ACS
	if nr.acsConnectionDelay > 0 {
		log.Printf("[Atraso] Atrasando conexão com o ACS em %v", nr.acsConnectionDelay)
		time.Sleep(nr.acsConnectionDelay)
	}

	// Conecta ao ACS
	serverConn, err := net.Dial("tcp", nr.acsIP+":"+nr.acsPort)
	if err != nil {
		log.Printf("[Erro] Falha ao conectar ao ACS %s:%s - %v", nr.acsIP, nr.acsPort, err)
		return
	}
	defer serverConn.Close()

	log.Printf("[Roteamento] %s -> ACS (%s:%s)", clientAddr, nr.acsIP, nr.acsPort)

	// Cria canais para sincronização
	done := make(chan struct{})

	// Device -> ACS
	go nr.forwardTraffic(clientConn, serverConn, "Device->ACS", done)

	// ACS -> Device
	go nr.forwardTraffic(serverConn, clientConn, "ACS->Device", done)

	// Aguarda finalização de uma das goroutines
	<-done
	log.Printf("[Desconexão] Conexão fechada: %s. Duração: %v", clientAddr, time.Since(startTime))
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
			log.Printf("[%s] Erro ao ler: %v", direction, err)
			return
		}

		if n == 0 {
			return
		}

		// Simula perda de pacotes
		if nr.shouldDropPacket() {
			nr.mu.Lock()
			nr.requestsDropped++
			nr.mu.Unlock()
			log.Printf("[%s] Pacote descartado (simulação)", direction)
			continue
		}

		// Simula latência
		if nr.latency > 0 && strings.Contains(direction, "Device") {
			time.Sleep(nr.latency)
		} else if nr.returnLatency > 0 && strings.Contains(direction, "ACS") {
			time.Sleep(nr.returnLatency)
		}

		// Simula limitação de largura de banda
		if nr.bandwidthKbps > 0 {
			delayMs := time.Duration((n*8)/(nr.bandwidthKbps/1000)) * time.Millisecond
			time.Sleep(delayMs)
		}

		// Envia para destino
		_, err = dst.Write(buffer[:n])
		if err != nil {
			log.Printf("[%s] Erro ao escrever: %v", direction, err)
			return
		}

		nr.logTraffic(direction, buffer[:n])
	}
}

// Decide aleatoriamente se deve descartar pacote
func (nr *NATRouter) shouldDropPacket() bool {
	if nr.packetLoss <= 0 {
		return false
	}
	return false // Implementação simplificada
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

	// Log de tipo de mensagem (HTTP, XML, etc)
	if len(data) > 0 {
		preview := string(data[:min(len(data), 50)])
		if strings.Contains(preview, "POST") || strings.Contains(preview, "HTTP") {
			log.Printf("[%s] HTTP | Tamanho: %d bytes", direction, len(data))
		} else if strings.Contains(preview, "<?xml") {
			log.Printf("[%s] XML | Tamanho: %d bytes", direction, len(data))
		} else {
			log.Printf("[%s] Dados | Tamanho: %d bytes", direction, len(data))
		}
	}
}

// Configurações via CLI
func (nr *NATRouter) SetLatency(ms int64) {
	nr.latency = time.Duration(ms) * time.Millisecond
	log.Printf("[Config] Latência ajustada para %v", nr.latency)
}

func (nr *NATRouter) SetReturnLatency(ms int64) {
	nr.returnLatency = time.Duration(ms) * time.Millisecond
	log.Printf("[Config] Latência de retorno ajustada para %v", nr.returnLatency)
}

func (nr *NATRouter) SetAcsConnectionDelay(ms int64) {
	nr.acsConnectionDelay = time.Duration(ms) * time.Millisecond
	log.Printf("[Config] Atraso de conexão com ACS ajustado para %v", nr.acsConnectionDelay)
}

func (nr *NATRouter) SetPacketLoss(percentage float64) {
	if percentage < 0 {
		percentage = 0
	}
	if percentage > 1 {
		percentage = 1
	}
	nr.packetLoss = percentage
	log.Printf("[Config] Perda de pacotes ajustada para %.2f%%", nr.packetLoss*100)
}

func (nr *NATRouter) SetBandwidth(kbps int) {
	nr.bandwidthKbps = kbps
	if kbps > 0 {
		log.Printf("[Config] Largura de banda limitada a %d Kbps", kbps)
	} else {
		log.Printf("[Config] Largura de banda ilimitada")
	}
}

func (nr *NATRouter) SetDeviceIP(ip, port string) {
	nr.deviceIP = ip
	nr.devicePort = port
	log.Printf("[Config] Device IP ajustado para %s:%s", ip, port)
}

func (nr *NATRouter) SetACSIP(ip, port string) {
	nr.acsIP = ip
	nr.acsPort = port
	log.Printf("[Config] ACS IP ajustado para %s:%s", ip, port)
}

// Servidor HTTP para controle remoto
func (nr *NATRouter) StartControlAPI(controlPort string) {
	http.HandleFunc("/config/latency", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			var ms int64
			fmt.Sscanf(r.URL.Query().Get("ms"), "%d", &ms)
			nr.SetLatency(ms)
			fmt.Fprintf(w, "Latência ajustada para %d ms", ms)
		}
	})

	http.HandleFunc("/config/return-latency", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			var ms int64
			fmt.Sscanf(r.URL.Query().Get("ms"), "%d", &ms)
			nr.SetReturnLatency(ms)
			fmt.Fprintf(w, "Latência de retorno ajustada para %d ms", ms)
		}
	})

	http.HandleFunc("/config/acs-connection-delay", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			var ms int64
			fmt.Sscanf(r.URL.Query().Get("ms"), "%d", &ms)
			nr.SetAcsConnectionDelay(ms)
			fmt.Fprintf(w, "Atraso de conexão com ACS ajustado para %d ms", ms)
		}
	})

	http.HandleFunc("/config/packetloss", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			var loss float64
			fmt.Sscanf(r.URL.Query().Get("percent"), "%f", &loss)
			nr.SetPacketLoss(loss / 100)
			fmt.Fprintf(w, "Perda ajustada para %.2f%%", loss)
		}
	})

	http.HandleFunc("/config/bandwidth", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			var kbps int
			fmt.Sscanf(r.URL.Query().Get("kbps"), "%d", &kbps)
			nr.SetBandwidth(kbps)
			fmt.Fprintf(w, "Banda ajustada para %d Kbps", kbps)
		}
	})

	http.HandleFunc("/stats", func(w http.ResponseWriter, r *http.Request) {
		nr.mu.RLock()
		defer nr.mu.RUnlock()
		fmt.Fprintf(w, `
Estatísticas do NAT Router:
- Conexões ativas: %d
- Total de requisições: %d
- Total de respostas: %d
- Pacotes descartados: %d
- IP do Router: %s
- IP Público: %s
- Duração da última conexão: %v
- Tempo total de conexão: %v
`, nr.activeConns, nr.totalRequests, nr.totalResponses, nr.requestsDropped, nr.routerIP, nr.publicIP, nr.lastConnectionDuration, nr.totalConnectionTime)
	})

	log.Printf("[API] Controle iniciado em http://localhost:%s", controlPort)
	log.Fatal(http.ListenAndServe(":"+controlPort, nil))
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func main() {
	deviceIP := flag.String("device-ip", "127.0.0.1", "IP do simulador TR-069")
	devicePort := flag.String("device-port", "0", "Porta do simulador TR-069 (0 = não validar)")
	acsIP := flag.String("acs-ip", "127.0.0.1", "IP do ACS")
	acsPort := flag.String("acs-port", "9292", "Porta do ACS")
	listenPort := flag.String("listen-port", "9999", "Porta de escuta do NAT Router")
	controlPort := flag.String("control-port", "8888", "Porta da API de controle")
	latency := flag.Int64("latency-ms", 10, "Latência em ms")
	acsConnectionDelay := flag.Int64("acs-connection-delay-ms", 0, "Atraso para conectar ao ACS em ms")
	returnLatency := flag.Int64("return-latency-ms", 10, "Latência de retorno em ms")
	flag.Parse()

	router := NewNATRouter(*deviceIP, *devicePort, *acsIP, *acsPort, *listenPort)
	router.SetLatency(*latency)

	// Inicia API de controle em goroutine
	go router.StartControlAPI(*controlPort)
	router.SetAcsConnectionDelay(*acsConnectionDelay)
	router.SetReturnLatency(*returnLatency)

	// Inicia router NAT
	if err := router.Start(); err != nil {
		log.Fatalf("[Erro Fatal] %v", err)
	}
}
