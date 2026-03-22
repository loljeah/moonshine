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
		fmt.Fprintln(os.Stderr, "commands: toggle [clipboard|type], status, mode [clipboard|type],")
		fmt.Fprintln(os.Stderr, "          freespeech on|off|toggle, listen start|stop,")
		fmt.Fprintln(os.Stderr, "          device <name>, devices, settings [key [value]],")
		fmt.Fprintln(os.Stderr, "          logs [n], quit")
		os.Exit(1)
	}

	command := strings.Join(os.Args[1:], " ")
	isMultiLine := strings.HasPrefix(command, "logs") || strings.HasPrefix(command, "devices")

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

		// For multi-line responses, continue reading
		if isMultiLine {
			for scanner.Scan() {
				fmt.Println(scanner.Text())
			}
		}
	}
}
