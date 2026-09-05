package ssh

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"net"
	"strconv"
	"strings"
	"time"

	cryptossh "golang.org/x/crypto/ssh"
)

// ErrHostKeyMismatch means the server presented a different host key than the
// one recorded on first connection — possibly a rebuilt VPS, possibly an
// attack. The operator must clear the stored key deliberately.
var ErrHostKeyMismatch = errors.New("SSH host key does not match the key recorded on first connection")

// Target identifies one remote host. KnownHostKey is the authorized key line
// recorded on the first trusted connection; when empty the connection trusts
// and returns the presented key (trust-on-first-use), and every later
// connection enforces it.
type Target struct {
	Address      string
	Port         int
	User         string
	KnownHostKey string
}

// Result carries the outcome of one command execution.
type Result struct {
	Stdout   string
	Stderr   string
	ExitCode int
	HostKey  string // the presented host key, for first-use recording
}

// Client executes allowlisted commands over SSH. It opens one connection per
// call — Ship's V1 operations are infrequent enough that pooling would be
// speculative complexity.
type Client struct {
	DialTimeout time.Duration
}

func NewClient() *Client {
	return &Client{DialTimeout: 10 * time.Second}
}

// Run executes a rendered allowlist command. The command is passed as an exact
// argv-derived string built from the typed operation — never from user text.
// When stream is non-nil it receives each stdout line as it arrives.
func (client *Client) Run(ctx context.Context, target Target, signer cryptossh.Signer, command Command, stream func(line string)) (Result, error) {
	result := Result{ExitCode: -1}
	presented := ""
	hostKeyCallback := func(hostname string, remote net.Addr, key cryptossh.PublicKey) error {
		presented = strings.TrimSpace(string(cryptossh.MarshalAuthorizedKey(key)))
		if target.KnownHostKey != "" && presented != strings.TrimSpace(target.KnownHostKey) {
			return ErrHostKeyMismatch
		}
		return nil
	}

	port := target.Port
	if port == 0 {
		port = 22
	}
	address := net.JoinHostPort(target.Address, strconv.Itoa(port))
	configuration := &cryptossh.ClientConfig{
		User:            target.User,
		Auth:            []cryptossh.AuthMethod{cryptossh.PublicKeys(signer)},
		HostKeyCallback: hostKeyCallback,
		Timeout:         client.DialTimeout,
	}

	dialer := net.Dialer{Timeout: client.DialTimeout}
	rawConnection, err := dialer.DialContext(ctx, "tcp", address)
	if err != nil {
		return result, fmt.Errorf("dial %s: %w", address, err)
	}
	sshConnection, channels, requests, err := cryptossh.NewClientConn(rawConnection, address, configuration)
	if err != nil {
		rawConnection.Close()
		if errors.Is(err, ErrHostKeyMismatch) || strings.Contains(err.Error(), ErrHostKeyMismatch.Error()) {
			return result, ErrHostKeyMismatch
		}
		return result, fmt.Errorf("SSH handshake with %s: %w", address, err)
	}
	connection := cryptossh.NewClient(sshConnection, channels, requests)
	defer connection.Close()
	result.HostKey = presented

	session, err := connection.NewSession()
	if err != nil {
		return result, fmt.Errorf("open SSH session: %w", err)
	}
	defer session.Close()

	stdout, err := session.StdoutPipe()
	if err != nil {
		return result, err
	}
	var stderr strings.Builder
	session.Stderr = &stderr

	// The remote side receives one command line assembled from the fixed
	// program and quoted arguments of the typed operation.
	if err := session.Start(command.CommandLine()); err != nil {
		return result, fmt.Errorf("start remote command: %w", err)
	}

	// Cancel the session when the context ends so reads unblock.
	done := make(chan struct{})
	defer close(done)
	go func() {
		select {
		case <-ctx.Done():
			session.Close()
		case <-done:
		}
	}()

	var output strings.Builder
	scanner := bufio.NewScanner(stdout)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		line := scanner.Text()
		output.WriteString(line)
		output.WriteByte('\n')
		if stream != nil {
			stream(line)
		}
	}

	waitError := session.Wait()
	result.Stdout = output.String()
	result.Stderr = stderr.String()
	if waitError == nil {
		result.ExitCode = 0
		return result, nil
	}
	var exitError *cryptossh.ExitError
	if errors.As(waitError, &exitError) {
		result.ExitCode = exitError.ExitStatus()
		return result, nil
	}
	if ctx.Err() != nil {
		return result, ctx.Err()
	}
	return result, fmt.Errorf("remote command: %w", waitError)
}

// CommandLine renders the fixed argv as a single shell-safe command line for
// the SSH exec channel, single-quoting every argument.
func (command Command) CommandLine() string {
	parts := make([]string, 0, len(command.Args)+1)
	parts = append(parts, command.Program)
	for _, argument := range command.Args {
		parts = append(parts, "'"+strings.ReplaceAll(argument, "'", `'\''`)+"'")
	}
	return strings.Join(parts, " ")
}
