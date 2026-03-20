package main

import (
	"bufio"
	"fmt"
	"net"
	"os"
	"strings"

	"moonshine-daemon/internal/daemon"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: moonshine-ctl <command> [args...]")
		fmt.Fprintln(os.Stderr, "commands: toggle [clipboard|type], status, mode [clipboard|type], device <name>, devices, quit")
		os.Exit(1)
	}

	command := strings.Join(os.Args[1:], " ")

	conn, err := net.Dial("unix", daemon.SocketPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "cannot connect to daemon: %s\n", err)
		fmt.Fprintln(os.Stderr, "is moonshine-daemon running?")
		os.Exit(1)
	}
	defer conn.Close()

	fmt.Fprintln(conn, command)

	scanner := bufio.NewScanner(conn)
	if scanner.Scan() {
		line := scanner.Text()
		fmt.Println(line)

		if strings.HasPrefix(line, "ERR") {
			os.Exit(1)
		}
	}
}
