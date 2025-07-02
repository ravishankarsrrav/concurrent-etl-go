package main

import (
	"fmt"
	"log"
	"os"
	"time"

	"github.com/valyala/fasthttp"
)

func main() {
	// Setup logging to file
	logFile, err := os.OpenFile("mock_server.log", os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0666)
	if err != nil {
		log.Fatal("Cannot create log file:", err)
	}
	defer logFile.Close()
	log.SetOutput(logFile)

	// Create request router
	requestHandler := func(ctx *fasthttp.RequestCtx) {
		path := string(ctx.Path())
		method := string(ctx.Method())

		switch {
		case path == "/load" && method == fasthttp.MethodPost:
			handleLoad(ctx)
		case path == "/health" && method == fasthttp.MethodGet:
			handleHealth(ctx)
		default:
			ctx.Error("Unsupported path", fasthttp.StatusNotFound)
		}
	}

	fmt.Println("Mock API server started at http://localhost:8080")
	log.Println("Mock API server started at http://localhost:8080")

	// Start server
	if err := fasthttp.ListenAndServe(":8080", requestHandler); err != nil {
		log.Fatalf("Error starting server: %v", err)
	}
}

func handleHealth(ctx *fasthttp.RequestCtx) {
	ctx.SetContentType("application/json")
	ctx.SetStatusCode(fasthttp.StatusOK)
	ctx.SetBody([]byte(`{"status":"ok"}`))
}

func handleLoad(ctx *fasthttp.RequestCtx) {
	body := ctx.PostBody()
	bodySize := len(body)

	log.Printf("Received POST /load with size %d bytes", bodySize)
	log.Printf("Body Preview: %s", previewBody(body, 500))

	// Optional: Simulate processing delay
	time.Sleep(2 * time.Second)

	ctx.SetContentType("application/json")
	ctx.SetStatusCode(fasthttp.StatusOK)
	ctx.SetBody([]byte(`{"status":"success"}`))
}

func previewBody(body []byte, max int) string {
	if len(body) <= max {
		return string(body)
	}
	return string(body[:max]) + "...(truncated)"
}
