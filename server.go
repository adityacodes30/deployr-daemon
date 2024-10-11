package main

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
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

	http.HandleFunc("/deploy", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintln(w, "Deployment initiated")

		go func() {
			cmd := exec.Command("sudo", "/bin/bash", "/.deployr/deployr-daemon.sh", nextjsRepoURL)
			output, err := cmd.CombinedOutput()

			if err != nil {
				log.Printf("Error deploy script: %v\n", err)
				log.Printf("Script output: %s\n", string(output))

				http.Error(w, "Deployment failed", http.StatusInternalServerError)
				return
			}

			log.Printf("Deployment successful.\n")
			log.Printf("Script output: %s\n", string(output))
			fmt.Fprintln(w, "Deployment completed")
		}()
	})

	log.Println("Server running on :6213")
	if err := http.ListenAndServe(":6213", nil); err != nil {
		log.Fatal("Error starting server:", err)
	}
}
