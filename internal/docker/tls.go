// Copyright (C) 2026 Suleyman Poyraz
// SPDX-License-Identifier: GPL-3.0-or-later

package docker

import (
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

// TLSFromEnv returns the TLS configuration selected with the environment
// variables of the docker CLI: DOCKER_TLS_VERIFY verifies the daemon with
// the certificates in DOCKER_CERT_PATH (default ~/.docker), DOCKER_TLS uses
// TLS without verification. It returns nil when TLS is not requested.
func TLSFromEnv() (*tls.Config, error) {
	verify := os.Getenv("DOCKER_TLS_VERIFY") != ""
	if !verify && os.Getenv("DOCKER_TLS") == "" {
		return nil, nil
	}
	dir := os.Getenv("DOCKER_CERT_PATH")
	if dir == "" {
		var err error
		if dir, err = configDir(); err != nil {
			return nil, err
		}
	}
	return loadTLS(dir, !verify)
}

// loadTLS builds a TLS configuration from ca.pem, cert.pem and key.pem in
// dir; each file is optional.
func loadTLS(dir string, skipVerify bool) (*tls.Config, error) {
	cfg := &tls.Config{MinVersion: tls.VersionTLS12, InsecureSkipVerify: skipVerify} //nolint:gosec // requested by the user
	ca, err := os.ReadFile(filepath.Join(dir, "ca.pem"))
	switch {
	case err == nil:
		pool := x509.NewCertPool()
		if !pool.AppendCertsFromPEM(ca) {
			return nil, fmt.Errorf("docker TLS: no certificate in %s", filepath.Join(dir, "ca.pem"))
		}
		cfg.RootCAs = pool
	case !errors.Is(err, fs.ErrNotExist):
		return nil, fmt.Errorf("docker TLS: %w", err)
	}
	certFile, keyFile := filepath.Join(dir, "cert.pem"), filepath.Join(dir, "key.pem")
	if _, err := os.Stat(certFile); err == nil {
		pair, err := tls.LoadX509KeyPair(certFile, keyFile)
		if err != nil {
			return nil, fmt.Errorf("docker TLS: %w", err)
		}
		cfg.Certificates = []tls.Certificate{pair}
	}
	return cfg, nil
}
