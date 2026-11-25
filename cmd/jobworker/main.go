// Command jobworker runs the job server or talks to it over gRPC.
//
//	jobworker serve                     start the gRPC + HTTP server
//	jobworker run [-name N] -- CMD...   submit a job (add -follow to stream output)
//	jobworker list                      list jobs
//	jobworker get ID                    show one job
//	jobworker logs ID                   stream a job's output until it finishes
//	jobworker stop ID                   stop a pending or running job
//	jobworker delete ID                 delete a finished job and its log
package main

import (
	"fmt"
	"os"
)

const usage = `Usage: jobworker <command> [flags] [args]

Server:
  serve                    Start the gRPC and HTTP servers

Client (talks to the gRPC server):
  run [-name N] [-follow] -- CMD...
                           Submit a shell command
  list                     List jobs, newest first
  get ID                   Show one job
  logs ID                  Stream a job's output until it finishes
  stop ID                  Stop a pending or running job
  delete ID                Delete a finished job and its log

Run "jobworker <command> -h" for the flags of a command.
`

func main() {
	if len(os.Args) < 2 {
		fmt.Fprint(os.Stderr, usage)
		os.Exit(2)
	}
	cmd, args := os.Args[1], os.Args[2:]
	var err error
	switch cmd {
	case "serve":
		err = serve(args)
	case "run", "list", "get", "logs", "stop", "delete":
		err = client(cmd, args)
	case "-h", "-help", "--help", "help":
		fmt.Print(usage)
		return
	default:
		fmt.Fprintf(os.Stderr, "unknown command %q\n\n%s", cmd, usage)
		os.Exit(2)
	}
	if err != nil {
		if e, ok := err.(exitError); ok {
			os.Exit(int(e))
		}
		fmt.Fprintln(os.Stderr, "jobworker:", err)
		os.Exit(1)
	}
}

// exitError makes the process exit with the given code without printing.
type exitError int

func (e exitError) Error() string { return fmt.Sprintf("exit status %d", int(e)) }
