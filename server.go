package main

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"sync"
	"time"
)

type JobStatus struct {
	ID        string
	Status    string // "running", "completed", "failed"
	Output    string
	Timestamp time.Time
}

var (
	jobStatuses = make(map[string]*JobStatus)
	mu          sync.Mutex
)

func main() {
	if len(os.Args) < 2 {
		log.Fatal("Error: Missing argument.")
	}
	nextjsRepoURL := os.Args[1]

	logFile, err := os.OpenFile("/var/log/go-server.log", os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666)
	if err != nil {
		log.Fatal("Error opening log file:", err)
	}
	defer logFile.Close()
	log.SetOutput(logFile)

	runInitialDeployment(nextjsRepoURL)

	http.HandleFunc("/deploy", handleDeploy(nextjsRepoURL))
	http.HandleFunc("/status", handleStatus)

	log.Println("Server running on :6213")
	if err := http.ListenAndServe(":6213", nil); err != nil {
		log.Fatal("Error starting server:", err)
	}
}

func handleDeploy(nextjsRepoURL string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		jobID := fmt.Sprintf("%d", time.Now().UnixNano())

		mu.Lock()
		jobStatuses[jobID] = &JobStatus{
			ID:        jobID,
			Status:    "running",
			Timestamp: time.Now(),
		}
		mu.Unlock()

		fmt.Fprintf(w, "Deployment initiated. Job ID: %s\n", jobID)

		go runDeploymentScript(jobID, nextjsRepoURL)
	}
}

func handleStatus(w http.ResponseWriter, r *http.Request) {
	jobID := r.URL.Query().Get("job_id")
	if jobID == "" {
		http.Error(w, "Missing job_id parameter", http.StatusBadRequest)
		return
	}

	mu.Lock()
	status, exists := jobStatuses[jobID]
	mu.Unlock()

	if !exists {
		http.Error(w, "Invalid job ID", http.StatusNotFound)
		return
	}

	fmt.Fprintf(w, "Job ID: %s\nStatus: %s\nOutput: %s\n", status.ID, status.Status, status.Output)
}

func runInitialDeployment(nextjsRepoURL string) {
	log.Println("Running initial deployment...")

	jobID := "1"
	mu.Lock()
	jobStatuses[jobID] = &JobStatus{
		ID:        jobID,
		Status:    "running",
		Timestamp: time.Now(),
	}
	mu.Unlock()

	go runDeploymentScript(jobID, nextjsRepoURL)
}

func runDeploymentScript(jobID, nextjsRepoURL string) {
	cmd := exec.Command("sudo", "/bin/bash", "/.deployr/deployr-daemon.sh", nextjsRepoURL)
	output, err := cmd.CombinedOutput()

	mu.Lock()
	defer mu.Unlock()

	status := jobStatuses[jobID]
	if err != nil {
		status.Status = "failed"
		status.Output = fmt.Sprintf("Error: %v\nOutput: %s", err, string(output))
		log.Printf("Deployment failed for job %s: %v\n", jobID, err)
	} else {
		status.Status = "completed"
		status.Output = string(output)
		log.Printf("Deployment successful for job %s.\n", jobID)
	}
}
