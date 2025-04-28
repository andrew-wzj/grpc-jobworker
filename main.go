package main

import (
	"bufio"
	"fmt"
	"jobrunner/jobworker"
	"os"
	"strings"
	"time"
)

func main() {
	// Create a new JobWorker
	worker := jobworker.NewJobWorker()

	// Prompt user to input a command
	reader := bufio.NewReader(os.Stdin)
	fmt.Print("Enter a Linux command to run: ")
	cmdStr, _ := reader.ReadString('\n')
	cmdStr = strings.TrimSpace(cmdStr) // clean newline

	// Run the command
	sessionID, err := worker.Run(cmdStr)
	if err != nil {
		fmt.Println("Failed to start job:", err)
		return
	}

	fmt.Println("✅ Job started! Session ID:", sessionID)

	// Polling the job status every 1 second
	for {
		status, exists := worker.GetStatus(sessionID)
		if !exists {
			fmt.Println("❌ Job not found!")
			break
		}

		fmt.Println("📣 Current Status:", status)
		if status == "Completed" || status == "Failed" {
			fmt.Println("🏁 Job finished with status:", status)
			break
		}
		time.Sleep(1 * time.Second)
	}
}
