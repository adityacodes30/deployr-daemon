package main

import (
	"crypto"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"errors"
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

type DeployRequest struct {
	Message   string `json:"message"`
	Signature string `json:"signature"`
}

var (
	jobStatuses     = make(map[string]*JobStatus)
	mu              sync.Mutex
	parsedPublicKey *rsa.PublicKey // Store the decoded RSA public key
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

	err = loadPublicKey()
	if err != nil {
		log.Fatal("Error loading public key:", err)
	}

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
		var req DeployRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "Invalid request payload", http.StatusBadRequest)
			return
		}

		// Verify the message with the public key
		err := verifySignature(req.Message, req.Signature)
		if err != nil {
			http.Error(w, "Unauthorized: "+err.Error(), http.StatusUnauthorized)
			return
		}

		jobID := fmt.Sprintf("%d", time.Now().UnixNano())

		mu.Lock()
		jobStatuses[jobID] = &JobStatus{
			ID:        jobID,
			Status:    "running",
			Timestamp: time.Now(),
		}
		mu.Unlock()

		fmt.Fprintf(w, "%s", jobID)

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

	fmt.Fprintf(w, "%s", status.Status)
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

func loadPublicKey() error {
	pubKeyPEM := os.Getenv("DEPLOYR_PUBKEY")
	if pubKeyPEM == "" {
		return errors.New("public key not set in environment variable")
	}

	block, _ := pem.Decode([]byte(pubKeyPEM))
	if block == nil || block.Type != "PUBLIC KEY" {
		return errors.New("failed to decode PEM block containing public key")
	}

	pubKey, err := x509.ParsePKIXPublicKey(block.Bytes)
	if err != nil {
		return fmt.Errorf("failed to parse public key: %v", err)
	}

	rsaPubKey, ok := pubKey.(*rsa.PublicKey)
	if !ok {
		return errors.New("public key is not of type RSA")
	}

	parsedPublicKey = rsaPubKey
	return nil
}

func verifySignature(message, signature string) error {
	if parsedPublicKey == nil {
		return errors.New("public key not loaded")
	}

	messageHash := sha256.Sum256([]byte(message))

	sigBytes, err := base64.StdEncoding.DecodeString(signature)
	if err != nil {
		return fmt.Errorf("failed to decode signature: %v", err)
	}

	// RSA verification
	err = rsa.VerifyPKCS1v15(parsedPublicKey, crypto.SHA256, messageHash[:], sigBytes)
	if err != nil {
		return fmt.Errorf("signature verification failed: %v", err)
	}

	if message != "deployr" {
		return errors.New("message content verification failed")
	}

	return nil
}
