// Copyright (C) 2026 Suleyman Poyraz
// SPDX-License-Identifier: GPL-3.0-or-later

package docker

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/hex"
	"encoding/pem"
	"io"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestMain also acts as a fake ssh: with DTUI_SSH_SOCKET set, the test
// binary relays its standard streams to that socket, like
// "docker system dial-stdio" does on the remote host.
func TestMain(m *testing.M) {
	if sock := os.Getenv("DTUI_SSH_SOCKET"); sock != "" {
		if msg := os.Getenv("DTUI_SSH_FAIL"); msg != "" {
			os.Stderr.WriteString(msg + "\n")
			os.Exit(255)
		}
		if os.Getenv("DTUI_SSH_HANG") != "" {
			time.Sleep(10 * time.Second)
			os.Exit(0)
		}
		conn, err := net.Dial("unix", sock)
		if err != nil {
			os.Exit(1)
		}
		go func() { _, _ = io.Copy(conn, os.Stdin); conn.(*net.UnixConn).CloseWrite() }()
		_, _ = io.Copy(os.Stdout, conn)
		os.Exit(0)
	}
	os.Exit(m.Run())
}

func versionHandler(t *testing.T) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/version" {
			http.NotFound(w, r)
			return
		}
		writeJSON(w, map[string]string{"Version": "27.0.0"})
	})
}

func TestSSHArgs(t *testing.T) {
	u, _ := url.Parse("ssh://deploy@build.example.com:2222")
	args, err := sshArgs(u)
	if err != nil {
		t.Fatal(err)
	}
	want := "-o ConnectTimeout=30 -T -l deploy -p 2222 -- build.example.com docker system dial-stdio"
	if got := strings.Join(args, " "); got != want {
		t.Errorf("args = %q", got)
	}
	u, _ = url.Parse("ssh://host/some/path")
	if _, err := sshArgs(u); err == nil {
		t.Error("path accepted")
	}
}

func fakeSSH(t *testing.T, sock, fail string) *[]string {
	t.Helper()
	var got []string
	old := sshCommand
	sshCommand = func(args ...string) *exec.Cmd {
		got = args
		cmd := exec.Command(os.Args[0])
		cmd.Env = append(os.Environ(), "DTUI_SSH_SOCKET="+sock, "DTUI_SSH_FAIL="+fail)
		return cmd
	}
	t.Cleanup(func() { sshCommand = old })
	return &got
}

func unixServer(t *testing.T, h http.Handler) string {
	t.Helper()
	sock := filepath.Join(t.TempDir(), "d.sock")
	l, err := net.Listen("unix", sock)
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewUnstartedServer(h)
	srv.Listener = l
	srv.Start()
	t.Cleanup(srv.Close)
	return sock
}

func TestSSHTransport(t *testing.T) {
	sock := unixServer(t, versionHandler(t))
	args := fakeSSH(t, sock, "")
	c, err := NewClient("ssh://me@box")
	if err != nil {
		t.Fatal(err)
	}
	v, err := c.ServerVersion(context.Background())
	if err != nil || v.Version != "27.0.0" {
		t.Fatalf("Version = %+v, %v", v, err)
	}
	if !strings.Contains(strings.Join(*args, " "), "-l me -- box docker system dial-stdio") {
		t.Errorf("ssh args = %q", *args)
	}
}

func TestSSHError(t *testing.T) {
	fakeSSH(t, "unused", "me@box: Permission denied (publickey).")
	c, err := NewClient("ssh://me@box")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := c.ServerVersion(context.Background()); err == nil || !strings.Contains(err.Error(), "Permission denied (publickey)") {
		t.Errorf("err = %v", err)
	}
}

func TestSSHCancellation(t *testing.T) {
	old := sshCommand
	sshCommand = func(args ...string) *exec.Cmd {
		cmd := exec.Command(os.Args[0])
		cmd.Env = append(os.Environ(), "DTUI_SSH_SOCKET=unused", "DTUI_SSH_HANG=1")
		return cmd
	}
	t.Cleanup(func() { sshCommand = old })

	c, err := NewClient("ssh://me@box")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	start := time.Now()
	_, err = c.ServerVersion(ctx)
	if err == nil {
		t.Fatal("expected error from canceled context")
	}
	if elapsed := time.Since(start); elapsed > 2*time.Second {
		t.Fatalf("SSH dial took %v, expected cancellation around 100ms", elapsed)
	}
}

// writeCerts creates a CA, a server certificate for 127.0.0.1 and a client
// certificate, and writes the client files the CLI expects into dir.
func writeCerts(t *testing.T, dir string) *tls.Config {
	t.Helper()
	newKey := func() *ecdsa.PrivateKey {
		k, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
		if err != nil {
			t.Fatal(err)
		}
		return k
	}
	caKey := newKey()
	caTmpl := &x509.Certificate{
		SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "ca"},
		NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour),
		IsCA: true, BasicConstraintsValid: true, KeyUsage: x509.KeyUsageCertSign,
	}
	caDER, _ := x509.CreateCertificate(rand.Reader, caTmpl, caTmpl, &caKey.PublicKey, caKey)
	ca, _ := x509.ParseCertificate(caDER)
	issue := func(serial int64, usage x509.ExtKeyUsage, ip bool) (tls.Certificate, []byte, []byte) {
		k := newKey()
		tmpl := &x509.Certificate{
			SerialNumber: big.NewInt(serial), Subject: pkix.Name{CommonName: "leaf"},
			NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour),
			ExtKeyUsage: []x509.ExtKeyUsage{usage}, KeyUsage: x509.KeyUsageDigitalSignature,
		}
		if ip {
			tmpl.IPAddresses = []net.IP{net.ParseIP("127.0.0.1")}
		}
		der, _ := x509.CreateCertificate(rand.Reader, tmpl, ca, &k.PublicKey, caKey)
		keyDER, _ := x509.MarshalECPrivateKey(k)
		certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
		keyPEM := pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER})
		pair, _ := tls.X509KeyPair(certPEM, keyPEM)
		return pair, certPEM, keyPEM
	}
	serverPair, _, _ := issue(2, x509.ExtKeyUsageServerAuth, true)
	_, clientCert, clientKey := issue(3, x509.ExtKeyUsageClientAuth, false)
	os.WriteFile(filepath.Join(dir, "ca.pem"), pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: caDER}), 0o644)
	os.WriteFile(filepath.Join(dir, "cert.pem"), clientCert, 0o644)
	os.WriteFile(filepath.Join(dir, "key.pem"), clientKey, 0o600)
	pool := x509.NewCertPool()
	pool.AddCert(ca)
	return &tls.Config{Certificates: []tls.Certificate{serverPair}, ClientCAs: pool, ClientAuth: tls.RequireAndVerifyClientCert, MinVersion: tls.VersionTLS12}
}

func tlsServer(t *testing.T, certDir string) string {
	srv := httptest.NewUnstartedServer(versionHandler(t))
	srv.TLS = writeCerts(t, certDir)
	srv.StartTLS()
	t.Cleanup(srv.Close)
	return "tcp://" + strings.TrimPrefix(srv.URL, "https://")
}

func TestTLSFromEnv(t *testing.T) {
	certs := t.TempDir()
	host := tlsServer(t, certs)
	t.Setenv("DOCKER_HOST", host)
	t.Setenv("DOCKER_TLS_VERIFY", "1")
	t.Setenv("DOCKER_CERT_PATH", certs)
	c, err := NewClient("")
	if err != nil {
		t.Fatal(err)
	}
	if v, err := c.ServerVersion(context.Background()); err != nil || v.Version != "27.0.0" {
		t.Fatalf("Version = %+v, %v", v, err)
	}
	// Without the client certificate the daemon refuses the connection.
	t.Setenv("DOCKER_TLS_VERIFY", "")
	c, _ = NewClient("")
	if _, err := c.ServerVersion(context.Background()); err == nil {
		t.Error("plain HTTP to a TLS daemon succeeded")
	}
}

func TestTLSContext(t *testing.T) {
	cfgDir := t.TempDir()
	t.Setenv("DOCKER_CONFIG", cfgDir)
	t.Setenv("DOCKER_HOST", "")
	sum := sha256.Sum256([]byte("secure"))
	tlsDir := filepath.Join(cfgDir, "contexts", "tls", hex.EncodeToString(sum[:]), "docker")
	os.MkdirAll(tlsDir, 0o755)
	host := tlsServer(t, tlsDir)
	writeContext(t, cfgDir, "secure", host)

	cfg, err := ContextEndpoint("secure")
	if err != nil || cfg.TLS == nil || cfg.Context != "secure" {
		t.Fatalf("ContextEndpoint = %+v, %v", cfg, err)
	}
	c, err := NewClientWithConfig(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if v, err := c.ServerVersion(context.Background()); err != nil || v.Version != "27.0.0" {
		t.Fatalf("Version = %+v, %v", v, err)
	}
	if c.Context() != "secure" {
		t.Errorf("Context() = %q", c.Context())
	}
}

func TestDefaultPorts(t *testing.T) {
	c, _ := NewClientWithConfig(Config{Host: "tcp://example.com"})
	if c.base != "http://example.com:2375" {
		t.Errorf("plain base = %q", c.base)
	}
	c, _ = NewClientWithConfig(Config{Host: "tcp://example.com", TLS: &tls.Config{MinVersion: tls.VersionTLS12}})
	if c.base != "https://example.com:2376" {
		t.Errorf("TLS base = %q", c.base)
	}
}
