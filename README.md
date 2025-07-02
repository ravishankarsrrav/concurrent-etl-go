# 🚀 Go ETL Pipeline with Buffered Load & Mock API Server

A high-performance ETL pipeline written in Go, with a **mock-load-api-server** for testing. Supports buffered loading, failure handling, automatic retries, and resource profiling.

## 📁 Project Structure

```
.
├── etl/                        # ETL pipeline source code
│   ├── main.go                  # ETL application
│   ├── appliances.csv           # Input CSV file
│   ├── etl.log                  # Logs
│   ├── cpu.prof                 # CPU profile
│   ├── mem.prof                 # Memory profile
│   ├── buffer_failed_worker*.gz # Failed buffers (auto-managed)
│   └── README.md                # (Optional) ETL specific docs
│
├── mock-load-api-server/        # Mock API server source code
│   ├── main.go                  # Mock server
│   ├── mock_server.log          # Logs
│   └── README.md                # (Optional) API server docs
│
└── README.md                    # This documentation file
```

## 🎯 Features

### ✅ ETL Pipeline

- ⚙️ High-concurrency extraction (configurable)
- 🚀 Buffered load with multiple loader workers
- 🔁 Automatically retries failed loads from previous runs (`buffer_failed_workerX.json.gz`)
- 🗑️ Deletes failed buffers after successful ingestion
- 📝 Logs all activity (`etl.log`)
- 🧠 CPU and memory profiling (`cpu.prof`, `mem.prof`)

### ✅ Mock API Server

- 💨 Built with [`fasthttp`](https://github.com/valyala/fasthttp) for high performance
- 🌐 Endpoints:
  - `/load` for POST data ingestion
  - `/health` for readiness checks
- 🔧 Simulates API responses with optional processing delay
- 📝 Logs all requests (`mock_server.log`)

## 🚀 Quick Start

### 1️⃣ Clone the repository

```bash
git clone git@github.com:ravishankarsrrav/concurrent-etl-go.git
cd concurrent-etl-go
```

### 2️⃣ Build the ETL Pipeline

```bash
cd etl
go build -o etl
```

### 3️⃣ Build the Mock API Server

```bash
cd ../mock-load-api-server
go build -o mock_server
```

## ▶️ Run the Mock API Server

```bash
./mock_server
```

🟢 **Endpoints:**

| Endpoint  | Method | Description              |
|-----------|--------|--------------------------|
| `/load`   | POST   | Accepts data from ETL    |
| `/health` | GET    | Health check endpoint    |

Logs are written to `mock_server.log`.

## ▶️ Run the ETL Pipeline

```bash
cd etl
./etl
```

## 📑 Input CSV Format

Example `appliances.csv`:

```csv
192.168.0.1,Device-1
192.168.0.2,Device-2
192.168.0.3,Device-3
```

## ⚙️ Configuration

| Parameter         | Location | Description                                |
|-------------------|----------|---------------------------------------------|
| `extractWorkers`  | main.go  | Number of concurrent extract goroutines    |
| `loadWorkers`     | main.go  | Number of loader workers                   |
| `bufferThreshold` | main.go  | Number of records before buffer flush      |
| `apiEndpoint`     | main.go  | Target API URL                             |
| `apiAuthToken`    | main.go  | Authorization header for API               |

🔧 Update these in `etl/main.go`.

## 🔥 Profiling

Generates profiling files:

- `cpu.prof` – CPU usage profile
- `mem.prof` – Heap memory profile

Analyze using:

```bash
go tool pprof cpu.prof
go tool pprof mem.prof
```

## 📜 Logs

| Component          | File                      |
|--------------------|---------------------------|
| ETL Pipeline       | `etl/etl.log`             |
| Mock API Server    | `mock-load-api-server/mock_server.log` |

## 🏗️ Failed Buffer Handling

- On API failure, data is saved as:

```
buffer_failed_workerX.json.gz
```

- On the **next ETL run**, it will:
  - ✅ Detect these files
  - 🔁 Load them into the respective loader queue
  - 🗑️ Delete them **after successful queueing**

## 🚀 Sample Log Output

```log
[Loader-4] Successfully flushed 200 records
[Loader-7] API load failed: timeout. Saved buffer to file.
[Loader-7] Successfully flushed 200 records
[Extract] Completed extraction for Device-192.168.0.1
```

## 🧠 Mock Server Example

Check health:

```bash
curl http://localhost:8080/health
```

Send data manually:

```bash
curl -X POST http://localhost:8080/load      -H "Content-Type: application/json"      -d '[{"name":"device1","cpu_number":"0", ...}]'
```


## 👨‍💻 Author

**Ravishankar S R**  
*Chaos Engineer*


## 🏎️ Ready to scale your ETL chaos with Go! 🔥

## ✅ Folder Structure Summary

| Folder                    | Description                            |
|---------------------------|----------------------------------------|
| `/etl`                    | The ETL engine with buffering & retry |
| `/mock-load-api-server`    | Mock API server using fasthttp        |
