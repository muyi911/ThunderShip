package main

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/pem"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"shell_workflow/internal/core"
	"strconv"
	"testing"

	"github.com/pkg/sftp"
	"golang.org/x/crypto/ssh"
)

func probeSSH(t *testing.T) core.Server {
	t.Helper()
	_, key, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	signer, err := ssh.NewSignerFromKey(key)
	if err != nil {
		t.Fatal(err)
	}
	cfg := &ssh.ServerConfig{PasswordCallback: func(conn ssh.ConnMetadata, password []byte) (*ssh.Permissions, error) {
		if conn.User() != "test" || string(password) != "probe-test-password" {
			return nil, fmt.Errorf("invalid credentials")
		}
		return nil, nil
	}, PublicKeyCallback: func(conn ssh.ConnMetadata, key ssh.PublicKey) (*ssh.Permissions, error) {
		if conn.User() != "test" {
			return nil, fmt.Errorf("invalid user")
		}
		return nil, nil
	}}
	cfg.AddHostKey(signer)
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { listener.Close() })
	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			go func() {
				defer conn.Close()
				server, channels, requests, err := ssh.NewServerConn(conn, cfg)
				if err != nil {
					return
				}
				defer server.Close()
				go ssh.DiscardRequests(requests)
				for request := range channels {
					channel, requests, err := request.Accept()
					if err != nil {
						continue
					}
					go func() {
						defer channel.Close()
						for req := range requests {
							if req.Type != "subsystem" {
								req.Reply(false, nil)
								continue
							}
							req.Reply(true, nil)
							server, err := sftp.NewServer(channel)
							if err == nil {
								defer server.Close()
								_ = server.Serve()
							}
							return
						}
					}()
				}
			}()
		}
	}()
	host, port, _ := net.SplitHostPort(listener.Addr().String())
	portNumber, _ := strconv.Atoi(port)
	return core.Server{Host: host, Port: portNumber, User: "test", Auth: "password", Fingerprint: ssh.FingerprintSHA256(signer.PublicKey())}
}

func TestProbeConnectionDraftDoesNotSave(t *testing.T) {
	a := testApp(t)
	s := probeSSH(t)
	fingerprint := s.Fingerprint
	s.Fingerprint = ""
	result, err := a.ProbeConnection(s, "probe-test-password")
	if err != nil || result.OK || result.Changed || result.Fingerprint != fingerprint {
		t.Fatalf("first connection: %+v, %v", result, err)
	}
	s.Fingerprint = result.Fingerprint
	result, err = a.ProbeConnection(s, "probe-test-password")
	if err != nil || !result.OK {
		t.Fatalf("confirmed draft: %+v, %v", result, err)
	}
	if _, err = a.ProbeConnection(s, "wrong-password"); err == nil {
		t.Fatal("draft password was ignored")
	}
	s.Auth, s.KeyPath = "key", filepath.Join(t.TempDir(), "missing-key")
	if _, err = a.ProbeConnection(s, ""); err == nil {
		t.Fatal("draft key path was ignored")
	}
	_, key, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	block, err := ssh.MarshalPrivateKeyWithPassphrase(key, "test", []byte("test-passphrase"))
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(s.KeyPath, pem.EncodeToMemory(block), 0600); err != nil {
		t.Fatal(err)
	}
	if result, err = a.ProbeConnection(s, "test-passphrase"); err != nil || !result.OK {
		t.Fatalf("draft encrypted key: %+v, %v", result, err)
	}
	if _, err = a.ProbeConnection(s, "wrong-passphrase"); err == nil {
		t.Fatal("incorrect private key passphrase was ignored")
	}
	s.Port = 0
	if _, err = a.ProbeConnection(s, ""); err == nil {
		t.Fatal("invalid port accepted")
	}
	c, err := a.GetConfig()
	if err != nil || len(c.Servers) != 0 {
		t.Fatal("probe changed live configuration", err)
	}
	if _, err = os.Stat(filepath.Join(a.dir, "config.json")); !os.IsNotExist(err) {
		t.Fatal("probe persisted configuration", err)
	}
}

func TestProbeSavedCredentialsAndHostPins(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Windows credential manager")
	}
	a := testApp(t)
	s := probeSSH(t)
	s.Name = "saved"
	c, err := a.SaveServer(s, "probe-test-password")
	if err != nil {
		t.Fatal(err)
	}
	s = c.Servers[0]
	defer core.DeleteSecret("server/" + s.ID)
	if result, err := a.ProbeConnection(s, ""); err != nil || !result.OK {
		t.Fatalf("saved credential fallback: %+v, %v", result, err)
	}
	if _, err := a.ProbeConnection(s, "wrong-password"); err == nil {
		t.Fatal("explicit draft credential did not override stored credential")
	}
	draft := s
	draft.User = "other-user"
	if _, err := a.ProbeConnection(draft, ""); err == nil {
		t.Fatal("changed user was accepted")
	}
	other := probeSSH(t)
	draft = s
	draft.Port = other.Port
	result, err := a.ProbeConnection(draft, "")
	if err != nil || result.OK || result.Changed || result.Fingerprint != other.Fingerprint {
		t.Fatalf("changed endpoint kept old pin: %+v, %v", result, err)
	}
	draft.Fingerprint = result.Fingerprint
	if _, err := a.ProbeConnection(draft, ""); err == nil {
		t.Fatal("saved credential was sent to changed endpoint")
	}
	if result, err := a.ProbeConnection(draft, "probe-test-password"); err != nil || !result.OK {
		t.Fatalf("changed endpoint with explicit credentials: %+v, %v", result, err)
	}
	a.config.Servers[0].Fingerprint = other.Fingerprint
	result, err = a.ProbeConnection(s, "")
	if err != nil || !result.Changed || result.OK {
		t.Fatalf("draft bypassed stored host pin: %+v, %v", result, err)
	}
	c, err = a.SaveServer(s, "")
	if err != nil || c.Servers[0].Fingerprint != other.Fingerprint {
		t.Fatal("save replaced existing host pin", err)
	}
	s.User = "changed-user"
	c, err = a.SaveServer(s, "")
	if err != nil || c.Servers[0].HasSecret {
		t.Fatal("credential retained after changing connection identity", err)
	}
	if _, err = core.GetSecret("server/" + s.ID); err == nil {
		t.Fatal("old connection credential was not removed")
	}
}

func TestSaveServerKeepsConfirmedDraftFingerprint(t *testing.T) {
	a := testApp(t)
	s := core.Server{Name: "new", Host: "localhost", Port: 22, User: "test", Auth: "password", Fingerprint: "SHA256:confirmed-draft-fingerprint"}
	c, err := a.SaveServer(s, "")
	if err != nil || c.Servers[0].Fingerprint != s.Fingerprint {
		t.Fatal("confirmed draft fingerprint was lost", err)
	}
	s = c.Servers[0]
	s.Port++
	c, err = a.SaveServer(s, "")
	if err != nil || c.Servers[0].Fingerprint != "" {
		t.Fatal("copied pin was retained after endpoint changed", err)
	}
}
