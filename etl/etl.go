package main

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"runtime/pprof"
	"strconv"
	"strings"
	"sync"
	"time"
)

//////////////////////////////////////////////////
// Data Structures
//////////////////////////////////////////////////

type Appliance struct {
	IP       string
	HostName string
}

type CpuStats struct {
	Name      string `json:"name"`
	Timestamp uint64 `json:"timestamp"`
	CPUNumber string `json:"cpu_number"`
	PIdle     string `json:"pIdle"`
	PUser     string `json:"pUser"`
	PSys      string `json:"pSys"`
	PIRQ      string `json:"pIRQ"`
	PNice     string `json:"pNice"`
}

type Indicator struct {
	Name  string  `json:"name"`
	Value float64 `json:"value"`
}

type DeviceData struct {
	Name       string      `json:"name"`
	CPUNumber  string      `json:"cpu_number"`
	Timestamp  uint64      `json:"timestamp"`
	Indicators []Indicator `json:"indicators"`
}

//////////////////////////////////////////////////
// Configuration
//////////////////////////////////////////////////

const (
	simulatedApiDelay = 6 * time.Second
	bufferThreshold   = 200
	apiEndpoint       = "http://localhost:8080/load"
	apiAuthToken      = "Bearer your-token-here"

	extractWorkers = 1000
	loadWorkers    = 10
)

//////////////////////////////////////////////////
// Global Variables
//////////////////////////////////////////////////

var (
	buffers   []*Buffer
	dataChan  []chan DeviceData
	logFile   *os.File
	startTime time.Time
)

type Buffer struct {
	sync.Mutex
	Data []DeviceData
}

//////////////////////////////////////////////////
// Main
//////////////////////////////////////////////////

func main() {
	setupLogging()
	defer logFile.Close()

	startTime = time.Now()

	startCPUProfile()
	defer stopCPUProfile()

	appliances, err := readAppliancesFromCSV("appliances.csv")
	if err != nil {
		log.Fatalf("Error reading CSV: %v", err)
	}

	initBuffers(loadWorkers)
	initChannels(loadWorkers)

	// Load failed buffers from previous runs
	loadFailedBuffers()

	logResourceUsage("Before ETL")

	// Start loader workers
	var loadWg sync.WaitGroup
	for i := 0; i < loadWorkers; i++ {
		loadWg.Add(1)
		go loadWorker(&loadWg, i)
	}

	// Start extract workers
	var extractWg sync.WaitGroup
	sem := make(chan struct{}, extractWorkers)

	for idx, appliance := range appliances {
		sem <- struct{}{}
		extractWg.Add(1)

		go func(ap Appliance, index int) {
			defer func() {
				<-sem
				extractWg.Done()
			}()

			// log.Printf("[Extract] Starting for %s (%s)", ap.HostName, ap.IP)

			cpuData, err := extractCpuData(ap)
			if err != nil {
				log.Printf("[Extract] Failed for %s: %v", ap.HostName, err)
				return
			}

			// log.Printf("[Extract] Completed for %s", ap.HostName)

			deviceData := transform(cpuData)
			targetWorker := index % loadWorkers

			dataChan[targetWorker] <- deviceData
		}(appliance, idx)
	}

	extractWg.Wait()

	// Close channels to signal loaders to finish
	for _, ch := range dataChan {
		close(ch)
	}

	loadWg.Wait()

	logResourceUsage("After ETL")
	log.Printf("Total execution time: %v", time.Since(startTime))

	writeMemoryProfile()
}

//////////////////////////////////////////////////
// Initialization
//////////////////////////////////////////////////

func initBuffers(count int) {
	buffers = make([]*Buffer, count)
	for i := 0; i < count; i++ {
		buffers[i] = &Buffer{
			Data: make([]DeviceData, 0, bufferThreshold),
		}
	}
}

func initChannels(count int) {
	dataChan = make([]chan DeviceData, count)
	for i := 0; i < count; i++ {
		dataChan[i] = make(chan DeviceData, 2000)
	}
}

//////////////////////////////////////////////////
// Extract
//////////////////////////////////////////////////

func extractCpuData(ap Appliance) (*CpuStats, error) {
	ctx, cancel := context.WithTimeout(context.Background(), simulatedApiDelay+2*time.Second)
	defer cancel()

	select {
	case <-time.After(simulatedApiDelay):
		return &CpuStats{
			Name:      ap.HostName,
			CPUNumber: "0",
			PIdle:     "95",
			PUser:     "3",
			PSys:      "1",
			PIRQ:      "0.5",
			PNice:     "0",
			Timestamp: uint64(time.Now().Unix()),
		}, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

//////////////////////////////////////////////////
// Transform
//////////////////////////////////////////////////

func transform(cpu *CpuStats) DeviceData {
	idle, _ := strconv.ParseFloat(cpu.PIdle, 64)
	pNice, _ := strconv.ParseFloat(cpu.PNice, 64)
	pUser, _ := strconv.ParseFloat(cpu.PUser, 64)
	pSys, _ := strconv.ParseFloat(cpu.PSys, 64)
	pIRQ, _ := strconv.ParseFloat(cpu.PIRQ, 64)

	indicators := []Indicator{
		{"utilization", 100 - idle},
		{"nice", pNice},
		{"user", pUser},
		{"system", pSys},
		{"irq", pIRQ},
	}

	return DeviceData{
		Name:       cpu.Name,
		CPUNumber:  cpu.CPUNumber,
		Timestamp:  cpu.Timestamp,
		Indicators: indicators,
	}
}

//////////////////////////////////////////////////
// Load Worker
//////////////////////////////////////////////////

func loadWorker(wg *sync.WaitGroup, workerID int) {
	defer wg.Done()

	buffer := buffers[workerID]
	ch := dataChan[workerID]

	for item := range ch {
		buffer.Lock()
		buffer.Data = append(buffer.Data, item)

		if len(buffer.Data) >= bufferThreshold {
			flushBuffer(buffer, workerID)
		}
		buffer.Unlock()
	}

	// Final flush
	buffer.Lock()
	if len(buffer.Data) > 0 {
		flushBuffer(buffer, workerID)
	}
	buffer.Unlock()
}

func flushBuffer(buffer *Buffer, workerID int) {
	toSend := make([]DeviceData, len(buffer.Data))
	copy(toSend, buffer.Data)

	err := sendToAPI(toSend)
	if err != nil {
		log.Printf("[Loader-%d] API load failed: %v. Saving buffer.", workerID, err)
		saveBufferToFile(toSend, fmt.Sprintf("buffer_failed_worker%d", workerID))
	} else {
		log.Printf("[Loader-%d] Successfully flushed %d records", workerID, len(toSend))
	}

	buffer.Data = nil
}

//////////////////////////////////////////////////
// Send to API
//////////////////////////////////////////////////

func sendToAPI(data []DeviceData) error {
	payload, _ := json.Marshal(data)

	req, err := http.NewRequest("POST", apiEndpoint, bytes.NewReader(payload))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", apiAuthToken)
	req.Header.Set("Content-Type", "application/json")

	client := http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 200 && resp.StatusCode <= 299 {
		return nil
	}
	body, _ := io.ReadAll(resp.Body)
	return fmt.Errorf("API error: %s", string(body))
}

//////////////////////////////////////////////////
// Failed Buffer Loader
//////////////////////////////////////////////////

func loadFailedBuffers() {
	files, err := filepath.Glob("buffer_failed_worker*.json.gz")
	if err != nil {
		log.Printf("Error scanning failed buffer files: %v", err)
		return
	}

	for _, file := range files {
		log.Printf("Reloading failed buffer: %s", file)

		dataList, err := readBufferFromFile(file)
		if err != nil {
			log.Printf("Failed to read %s: %v", file, err)
			continue
		}

		workerID := extractWorkerID(file)

		for _, data := range dataList {
			dataChan[workerID] <- data
		}

		err = os.Remove(file)
		if err != nil {
			log.Printf("Failed to delete %s: %v", file, err)
		} else {
			log.Printf("Deleted failed buffer file: %s", file)
		}
	}
}

func readBufferFromFile(filePath string) ([]DeviceData, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	gzReader, err := gzip.NewReader(file)
	if err != nil {
		return nil, err
	}
	defer gzReader.Close()

	var data []DeviceData
	decoder := json.NewDecoder(gzReader)
	err = decoder.Decode(&data)
	return data, err
}

func extractWorkerID(fileName string) int {
	base := filepath.Base(fileName)
	parts := strings.Split(strings.TrimSuffix(base, ".json.gz"), "worker")
	if len(parts) != 2 {
		return 0
	}
	id, err := strconv.Atoi(parts[1])
	if err != nil {
		return 0
	}
	return id
}

//////////////////////////////////////////////////
// Buffer Save
//////////////////////////////////////////////////

func saveBufferToFile(data []DeviceData, filename string) {
	file, err := os.Create(filename + ".json.gz")
	if err != nil {
		log.Printf("Failed to create file: %v", err)
		return
	}
	defer file.Close()

	gzipWriter := gzip.NewWriter(file)
	defer gzipWriter.Close()

	encoder := json.NewEncoder(gzipWriter)
	err = encoder.Encode(data)
	if err != nil {
		log.Printf("Failed to encode buffer: %v", err)
	}
}

//////////////////////////////////////////////////
// CSV Reader
//////////////////////////////////////////////////

func readAppliancesFromCSV(filePath string) ([]Appliance, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	r := csv.NewReader(file)
	records, err := r.ReadAll()
	if err != nil {
		return nil, err
	}

	var appliances []Appliance
	for i, rec := range records {
		if len(rec) < 2 {
			log.Printf("Skipping invalid line %d", i+1)
			continue
		}
		appliances = append(appliances, Appliance{
			IP:       rec[0],
			HostName: rec[1],
		})
	}
	return appliances, nil
}

//////////////////////////////////////////////////
// Logging & Profiling
//////////////////////////////////////////////////

func setupLogging() {
	var err error
	logFile, err = os.OpenFile("etl.log", os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0666)
	if err != nil {
		log.Fatal("Cannot create log file:", err)
	}
	log.SetOutput(logFile)
}

var cpuProfile *os.File

func startCPUProfile() {
	var err error
	cpuProfile, err = os.Create("cpu.prof")
	if err == nil {
		pprof.StartCPUProfile(cpuProfile)
	}
}

func stopCPUProfile() {
	if cpuProfile != nil {
		pprof.StopCPUProfile()
		cpuProfile.Close()
	}
}

func writeMemoryProfile() {
	memProfile, err := os.Create("mem.prof")
	if err == nil {
		defer memProfile.Close()
		pprof.WriteHeapProfile(memProfile)
		log.Println("Memory profile written to mem.prof")
	}
}

func logResourceUsage(phase string) {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)

	cpuCount := runtime.NumCPU()
	goroutines := runtime.NumGoroutine()

	log.Printf("========= [%s] Resource Usage =========", phase)
	log.Printf("CPU Cores: %d", cpuCount)
	log.Printf("Active Goroutines: %d", goroutines)
	log.Printf("Memory Alloc = %v MiB", bToMb(m.Alloc))
	log.Printf("Total Memory Alloc = %v MiB", bToMb(m.TotalAlloc))
	log.Printf("System Memory = %v MiB", bToMb(m.Sys))
	log.Printf("NumGC = %v", m.NumGC)
	log.Printf("========================================")
}

func bToMb(b uint64) uint64 {
	return b / 1024 / 1024
}
