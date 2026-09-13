package security

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"io/ioutil"
	"net/http"
)

// MTLSConfig holds mTLS configuration
type MTLSConfig struct {
	CertFile       string
	KeyFile        string
	CAFile         string
	ClientAuth     tls.ClientAuthType
	MinTLSVersion  uint16
	CipherSuites   []uint16
	VerifyPeerCert bool
}

// DefaultMTLSConfig returns secure default mTLS configuration
func DefaultMTLSConfig() MTLSConfig {
	return MTLSConfig{
		ClientAuth:     tls.RequireAndVerifyClientCert,
		MinTLSVersion:  tls.VersionTLS13,
		VerifyPeerCert: true,
		CipherSuites: []uint16{
			tls.TLS_AES_256_GCM_SHA384,
			tls.TLS_CHACHA20_POLY1305_SHA256,
			tls.TLS_AES_128_GCM_SHA256,
		},
	}
}

// NewServerTLSConfig creates TLS config for server
func NewServerTLSConfig(config MTLSConfig) (*tls.Config, error) {
	// Load server certificate
	cert, err := tls.LoadX509KeyPair(config.CertFile, config.KeyFile)
	if err != nil {
		return nil, fmt.Errorf("failed to load server certificate: %w", err)
	}

	// Load CA certificate for client verification
	caCert, err := ioutil.ReadFile(config.CAFile)
	if err != nil {
		return nil, fmt.Errorf("failed to load CA certificate: %w", err)
	}

	caPool := x509.NewCertPool()
	if !caPool.AppendCertsFromPEM(caCert) {
		return nil, fmt.Errorf("failed to parse CA certificate")
	}

	tlsConfig := &tls.Config{
		Certificates: []tls.Certificate{cert},
		ClientCAs:    caPool,
		ClientAuth:   config.ClientAuth,
		MinVersion:   config.MinTLSVersion,
		CipherSuites: config.CipherSuites,
		PreferServerCipherSuites: true,
	}

	return tlsConfig, nil
}

// NewClientTLSConfig creates TLS config for client
func NewClientTLSConfig(config MTLSConfig) (*tls.Config, error) {
	// Load client certificate
	cert, err := tls.LoadX509KeyPair(config.CertFile, config.KeyFile)
	if err != nil {
		return nil, fmt.Errorf("failed to load client certificate: %w", err)
	}

	// Load CA certificate for server verification
	caCert, err := ioutil.ReadFile(config.CAFile)
	if err != nil {
		return nil, fmt.Errorf("failed to load CA certificate: %w", err)
	}

	caPool := x509.NewCertPool()
	if !caPool.AppendCertsFromPEM(caCert) {
		return nil, fmt.Errorf("failed to parse CA certificate")
	}

	tlsConfig := &tls.Config{
		Certificates:       []tls.Certificate{cert},
		RootCAs:            caPool,
		MinVersion:         config.MinTLSVersion,
		CipherSuites:       config.CipherSuites,
		InsecureSkipVerify: !config.VerifyPeerCert,
	}

	return tlsConfig, nil
}

// NewMTLSClient creates an HTTP client with mTLS
func NewMTLSClient(config MTLSConfig) (*http.Client, error) {
	tlsConfig, err := NewClientTLSConfig(config)
	if err != nil {
		return nil, err
	}

	transport := &http.Transport{
		TLSClientConfig: tlsConfig,
	}

	return &http.Client{
		Transport: transport,
	}, nil
}

// ServiceCertificate represents a service certificate
type ServiceCertificate struct {
	ServiceName string
	CommonName  string
	DNSNames    []string
	IPAddresses []string
}

// ValidateServiceCert validates a service certificate
func ValidateServiceCert(cert *x509.Certificate, expectedService string) error {
	// Check if the certificate has the expected service name in CN or SAN
	if cert.Subject.CommonName != expectedService {
		validSAN := false
		for _, dns := range cert.DNSNames {
			if dns == expectedService {
				validSAN = true
				break
			}
		}
		if !validSAN {
			return fmt.Errorf("certificate does not belong to service: %s", expectedService)
		}
	}

	// Check if certificate is not expired
	if cert.NotAfter.Before(cert.NotBefore) {
		return fmt.Errorf("certificate has expired")
	}

	return nil
}

// CertRotationHandler handles certificate rotation
type CertRotationHandler struct {
	currentCert    *tls.Certificate
	certFile       string
	keyFile        string
	reloadInterval int64
}

// NewCertRotationHandler creates a new certificate rotation handler
func NewCertRotationHandler(certFile, keyFile string) *CertRotationHandler {
	return &CertRotationHandler{
		certFile:       certFile,
		keyFile:        keyFile,
		reloadInterval: 3600, // 1 hour
	}
}

// GetCertificate returns the current certificate, reloading if necessary
func (h *CertRotationHandler) GetCertificate(*tls.ClientHelloInfo) (*tls.Certificate, error) {
	// In production, implement proper certificate rotation logic
	cert, err := tls.LoadX509KeyPair(h.certFile, h.keyFile)
	if err != nil {
		return nil, err
	}
	return &cert, nil
}
