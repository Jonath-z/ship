package ssh

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"errors"
	"fmt"
	"net"
	"strconv"
	"strings"
	"testing"
	"time"

	cryptossh "golang.org/x/crypto/ssh"
)

// testSSHServer accepts connections and answers exec requests with a canned
// reply so the client's handshake, host-key, streaming, and exit paths can be
// exercised without a real VPS.
func testSSHServer(t *testing.T, reply string, exitCode int) (Target, cryptossh.Signer, string) {
	t.Helper()
	_, hostPrivate, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	hostSigner, err := cryptossh.NewSignerFromKey(hostPrivate)
	if err != nil {
		t.Fatal(err)
	}
	clientPublic, clientPrivate, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	clientSigner, err := cryptossh.NewSignerFromKey(clientPrivate)
	if err != nil {
		t.Fatal(err)
	}
	authorized, err := cryptossh.NewPublicKey(clientPublic)
	if err != nil {
		t.Fatal(err)
	}

	configuration := &cryptossh.ServerConfig{
		PublicKeyCallback: func(metadata cryptossh.ConnMetadata, key cryptossh.PublicKey) (*cryptossh.Permissions, error) {
			if string(key.Marshal()) == string(authorized.Marshal()) {
				return nil, nil
			}
			return nil, errors.New("unknown key")
		},
	}
	configuration.AddHostKey(hostSigner)

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { listener.Close() })

	go func() {
		for {
			rawConnection, err := listener.Accept()
			if err != nil {
				return
			}
			go func() {
				connection, channels, requests, err := cryptossh.NewServerConn(rawConnection, configuration)
				if err != nil {
					return
				}
				defer connection.Close()
				go cryptossh.DiscardRequests(requests)
				for newChannel := range channels {
					channel, channelRequests, err := newChannel.Accept()
					if err != nil {
						continue
					}
					go func() {
						for request := range channelRequests {
							if request.Type == "exec" {
								request.Reply(true, nil)
								fmt.Fprint(channel, reply)
								status := make([]byte, 4)
								status[3] = byte(exitCode)
								channel.SendRequest("exit-status", false, status)
								channel.Close()
							} else {
								request.Reply(false, nil)
							}
						}
					}()
				}
			}()
		}
	}()

	host, portText, _ := net.SplitHostPort(listener.Addr().String())
	port, _ := strconv.Atoi(portText)
	hostKey := strings.TrimSpace(string(cryptossh.MarshalAuthorizedKey(hostSigner.PublicKey())))
	return Target{Address: host, Port: port, User: "deploy"}, clientSigner, hostKey
}

func TestRunExecutesCommandAndRecordsHostKey(t *testing.T) {
	target, signer, hostKey := testSSHServer(t, "line one\nline two\n", 0)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	var streamed []string
	command, err := Render(Request{Operation: OperationDockerVersion})
	if err != nil {
		t.Fatal(err)
	}
	result, err := NewClient().Run(ctx, target, signer, command, func(line string) {
		streamed = append(streamed, line)
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.ExitCode != 0 || result.Stdout != "line one\nline two\n" {
		t.Fatalf("result = %#v", result)
	}
	if len(streamed) != 2 || streamed[0] != "line one" {
		t.Fatalf("streamed = %#v", streamed)
	}
	// Trust-on-first-use: the presented key is returned for recording.
	if result.HostKey != hostKey {
		t.Fatalf("host key = %q, want %q", result.HostKey, hostKey)
	}
}

func TestRunEnforcesRecordedHostKey(t *testing.T) {
	target, signer, _ := testSSHServer(t, "", 0)
	target.KnownHostKey = "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIGRlZmluaXRlbHkgbm90IHRoZSByaWdodCBrZXkhISE"
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	command, _ := Render(Request{Operation: OperationDockerVersion})
	_, err := NewClient().Run(ctx, target, signer, command, nil)
	if !errors.Is(err, ErrHostKeyMismatch) {
		t.Fatalf("error = %v, want host key mismatch", err)
	}
}

func TestRunReportsNonZeroExit(t *testing.T) {
	target, signer, _ := testSSHServer(t, "boom\n", 3)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	command, _ := Render(Request{Operation: OperationDiskUsage})
	result, err := NewClient().Run(ctx, target, signer, command, nil)
	if err != nil {
		t.Fatal(err)
	}
	if result.ExitCode != 3 {
		t.Fatalf("exit code = %d, want 3", result.ExitCode)
	}
}
