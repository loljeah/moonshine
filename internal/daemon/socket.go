package daemon

import (
	"bufio"
	"fmt"
	"log"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
)

const (
	// socketReadTimeout prevents DoS from clients that connect but never send data.
	socketReadTimeout = 5 * time.Second
	// maxCommandLength prevents memory exhaustion from very long commands.
	maxCommandLength = 4096
	// maxConcurrentConns limits simultaneous socket connections to prevent DoS.
	maxConcurrentConns = 10
)

const SocketPath = "/tmp/moonshine/moonshine.sock"

// SocketServer listens on a Unix socket for control commands.
type SocketServer struct {
	listener net.Listener
	daemon   *Daemon
	verbose  bool
	QuitCh   chan struct{}   // closed when "quit" command received
	connSem  chan struct{}   // semaphore limiting concurrent connections
}

// NewSocketServer creates and starts the Unix socket server.
func NewSocketServer(d *Daemon, verbose bool) (*SocketServer, error) {
	os.Remove(SocketPath)

	// Set restrictive umask before creating socket to prevent TOCTOU race
	oldUmask := syscall.Umask(0o077)
	ln, err := net.Listen("unix", SocketPath)
	syscall.Umask(oldUmask) // Restore original umask

	if err != nil {
		return nil, fmt.Errorf("listen %s: %w", SocketPath, err)
	}

	// Ensure permissions are correct (belt and suspenders)
	os.Chmod(SocketPath, 0o700)

	s := &SocketServer{
		listener: ln,
		daemon:   d,
		verbose:  verbose,
		QuitCh:   make(chan struct{}),
		connSem:  make(chan struct{}, maxConcurrentConns), // Limit concurrent connections
	}

	go s.acceptLoop()
	return s, nil
}

func (s *SocketServer) acceptLoop() {
	for {
		conn, err := s.listener.Accept()
		if err != nil {
			return
		}

		// Rate limit: acquire semaphore slot (non-blocking check first)
		select {
		case s.connSem <- struct{}{}:
			go func() {
				defer func() { <-s.connSem }() // Release slot when done
				s.handleConn(conn)
			}()
		default:
			// Too many connections, reject immediately
			conn.Close()
			if s.verbose {
				log.Printf("socket: rejected connection (too many concurrent)")
			}
		}
	}
}

func (s *SocketServer) handleConn(conn net.Conn) {
	defer conn.Close()

	// Set read deadline to prevent hanging on slow/malicious clients
	conn.SetReadDeadline(time.Now().Add(socketReadTimeout))

	scanner := bufio.NewScanner(conn)
	scanner.Buffer(make([]byte, maxCommandLength), maxCommandLength)

	if !scanner.Scan() {
		if err := scanner.Err(); err != nil {
			if s.verbose {
				log.Printf("socket read error: %v", err)
			}
		}
		return
	}

	line := strings.TrimSpace(scanner.Text())
	if s.verbose {
		log.Printf("socket: %q", line)
	}

	// Clear deadline for response write
	conn.SetReadDeadline(time.Time{})

	parts := strings.Fields(line)
	if len(parts) == 0 {
		fmt.Fprintln(conn, "ERR empty command")
		return
	}

	cmd := parts[0]
	args := parts[1:]

	switch cmd {
	case "toggle":
		if len(args) > 0 {
			s.daemon.SetMode(ParseOutputMode(args[0]))
		}
		text, err := s.daemon.Toggle()
		if err != nil {
			fmt.Fprintf(conn, "ERR %s\n", err)
		} else {
			fmt.Fprintf(conn, "OK %s\n", text)
		}

	case "status":
		state := s.daemon.GetState()
		fmt.Fprintf(conn, "OK %s\n", state)

	case "mode":
		if len(args) == 0 {
			fmt.Fprintf(conn, "OK %s\n", s.daemon.GetMode())
		} else {
			s.daemon.SetMode(ParseOutputMode(args[0]))
			fmt.Fprintf(conn, "OK %s\n", args[0])
		}

	case "device":
		if len(args) == 0 {
			fmt.Fprintln(conn, "ERR device name required")
			return
		}
		search := strings.Join(args, " ")
		matched, err := s.daemon.SwitchDevice(search)
		if err != nil {
			fmt.Fprintf(conn, "ERR %s\n", err)
		} else {
			fmt.Fprintf(conn, "OK %s\n", matched)
		}

	case "devices":
		devs, err := s.daemon.Devices()
		if err != nil {
			fmt.Fprintf(conn, "ERR %s\n", err)
			return
		}
		var lines []string
		for _, d := range devs {
			lines = append(lines, d.Description+" ("+d.NodeName+")")
		}
		fmt.Fprintf(conn, "OK %s\n", strings.Join(lines, "\n"))

	case "listen":
		if len(args) == 0 {
			fmt.Fprintln(conn, "ERR listen start|stop required")
			return
		}
		switch args[0] {
		case "start":
			if err := s.daemon.StartListening(); err != nil {
				fmt.Fprintf(conn, "ERR %s\n", err)
			} else {
				fmt.Fprintln(conn, "OK listening")
			}
		case "stop":
			s.daemon.StopListening()
			fmt.Fprintln(conn, "OK stopped")
		default:
			fmt.Fprintf(conn, "ERR listen start|stop, got %s\n", args[0])
		}

	case "freespeech":
		if len(args) == 0 {
			// Return current state
			if s.daemon.GetFreeSpeech() {
				fmt.Fprintln(conn, "OK on")
			} else {
				fmt.Fprintln(conn, "OK off")
			}
			return
		}
		switch args[0] {
		case "on", "start", "enable":
			s.daemon.SetFreeSpeech(true)
			fmt.Fprintln(conn, "OK on")
		case "off", "stop", "disable":
			s.daemon.SetFreeSpeech(false)
			fmt.Fprintln(conn, "OK off")
		case "toggle":
			enabled := !s.daemon.GetFreeSpeech()
			s.daemon.SetFreeSpeech(enabled)
			if enabled {
				fmt.Fprintln(conn, "OK on")
			} else {
				fmt.Fprintln(conn, "OK off")
			}
		default:
			fmt.Fprintf(conn, "ERR freespeech on|off|toggle, got %s\n", args[0])
		}

	case "logs":
		// Return recent log entries
		n := 50 // default lines
		if len(args) > 0 {
			if parsed, err := strconv.Atoi(args[0]); err == nil && parsed > 0 {
				n = parsed
				if n > 500 {
					n = 500 // cap at 500 lines
				}
			}
		}
		logPath := filepath.Join(os.Getenv("HOME"), ".local", "share", "moonshine", "daemon.log")
		data, err := os.ReadFile(logPath)
		if err != nil {
			fmt.Fprintf(conn, "ERR %s\n", err)
			return
		}
		lines := strings.Split(string(data), "\n")
		start := len(lines) - n
		if start < 0 {
			start = 0
		}
		recent := lines[start:]
		fmt.Fprintf(conn, "OK\n%s\n", strings.Join(recent, "\n"))

	case "settings":
		// Valid config keys (for validation)
		validKeys := map[string]bool{
			"DEVICE": true, "LANGUAGE": true, "AUTO_PUNCTUATION": true,
			"AUTO_CAPITALIZE": true, "FILLER_REMOVAL": true,
			"VOICE_COMMANDS": true, "NUMBER_FORMAT": true,
			"SILENCE_TIMEOUT": true,
		}

		if len(args) == 0 {
			// Return all settings
			all := s.daemon.cfg.All()
			var lines []string
			for k, v := range all {
				lines = append(lines, k+"="+v)
			}
			if len(lines) == 0 {
				fmt.Fprintln(conn, "OK (no settings)")
			} else {
				fmt.Fprintf(conn, "OK %s\n", strings.Join(lines, "\n"))
			}
			return
		}
		// Get or set a specific setting
		key := strings.ToUpper(args[0])
		if !validKeys[key] {
			fmt.Fprintf(conn, "ERR unknown setting: %s (valid: DEVICE, LANGUAGE, AUTO_PUNCTUATION, AUTO_CAPITALIZE, FILLER_REMOVAL, VOICE_COMMANDS, NUMBER_FORMAT)\n", key)
			return
		}
		if len(args) == 1 {
			// Get setting
			val := s.daemon.cfg.Get(key, "")
			if val == "" {
				fmt.Fprintf(conn, "OK %s=(unset)\n", key)
			} else {
				fmt.Fprintf(conn, "OK %s=%s\n", key, val)
			}
		} else {
			// Set setting
			val := strings.Join(args[1:], " ")
			s.daemon.cfg.Set(key, val)
			fmt.Fprintf(conn, "OK %s=%s\n", key, val)
		}

	case "scratch":
		result, err := s.daemon.ScratchThat()
		if err != nil {
			fmt.Fprintf(conn, "ERR %s\n", err)
		} else {
			fmt.Fprintf(conn, "OK %s\n", result)
		}

	case "quit":
		fmt.Fprintln(conn, "OK")
		close(s.QuitCh)

	default:
		fmt.Fprintf(conn, "ERR unknown command: %s\n", cmd)
	}
}

// Close shuts down the socket server and removes the socket file.
func (s *SocketServer) Close() {
	s.listener.Close()
	os.Remove(SocketPath)
}
