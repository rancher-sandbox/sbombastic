package database

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"net"
	"os"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	rootFolder       = "/database"
	serverCAFolder   = rootFolder + "/server-ca"
	clientCertFolder = rootFolder + "/client-cert"
	credentialFolder = rootFolder + "/credential"
	port             = credentialFolder + "/port"
	user             = credentialFolder + "/user"
	password         = credentialFolder + "/password"
	dbname           = credentialFolder + "/dbname"
	serverFQDN       = credentialFolder + "/host"
	serverCA         = serverCAFolder + "/ca.crt"
	clientCrt        = clientCertFolder + "/tls.crt"
	clientKey        = clientCertFolder + "/tls.key"
	mTLS             = "mTLS"
	TLS              = "TLS"
)

// readFileString reads a file and trims any whitespace/newlines.
func readFileString(path string) (string, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("read file: %w", err)
	}
	return strings.TrimSpace(string(b)), nil
}

type dbCreds struct {
	User       string
	Password   string
	Port       string
	DBName     string
	ServerFQDN string
}

func readDBCredentials() (*dbCreds, error) {
	userVal, err := readFileString(user)
	if err != nil {
		return nil, fmt.Errorf("failed to read user: %w", err)
	}
	passwordVal, err := readFileString(password)
	if err != nil {
		return nil, fmt.Errorf("failed to read password: %w", err)
	}
	portVal, err := readFileString(port)
	if err != nil {
		return nil, fmt.Errorf("failed to read port: %w", err)
	}
	dbnameVal, err := readFileString(dbname)
	if err != nil {
		return nil, fmt.Errorf("failed to read dbname: %w", err)
	}
	serverFQDNVal, err := readFileString(serverFQDN)
	if err != nil {
		return nil, fmt.Errorf("failed to read serverFQDN: %w", err)
	}
	return &dbCreds{
		User:       userVal,
		Password:   passwordVal,
		Port:       portVal,
		DBName:     dbnameVal,
		ServerFQDN: serverFQDNVal,
	}, nil
}

//nolint:nilnil // returning (nil, nil) is intentional: means no TLS config needed, not an error
func buildTLSConfig(tlsMode, serverFQDN string) (*tls.Config, error) {
	if tlsMode != mTLS && tlsMode != TLS {
		return nil, nil
	}

	rootCAs := x509.NewCertPool()
	caBytes, err := os.ReadFile(serverCA)
	if err != nil {
		return nil, fmt.Errorf("read CA file: %w", err)
	}
	if ok := rootCAs.AppendCertsFromPEM(caBytes); !ok {
		return nil, errors.New("failed to append CA cert")
	}

	if tlsMode == mTLS {
		clientCertKeyPair, err := tls.LoadX509KeyPair(clientCrt, clientKey)
		if err != nil {
			return nil, fmt.Errorf("load client cert key pair: %w", err)
		}
		return &tls.Config{
			ServerName:   serverFQDN,
			RootCAs:      rootCAs,
			Certificates: []tls.Certificate{clientCertKeyPair},
			MinVersion:   tls.VersionTLS12,
		}, nil
	}

	// tlsMode == TLS
	return &tls.Config{
		ServerName: serverFQDN,
		RootCAs:    rootCAs,
		MinVersion: tls.VersionTLS12,
	}, nil
}

func buildDSN(creds *dbCreds, sslMode string) string {
	return fmt.Sprintf(
		"postgres://%s:%s@%s/%s?sslmode=%s",
		creds.User, creds.Password, net.JoinHostPort(creds.ServerFQDN, creds.Port), creds.DBName, sslMode,
	)
}

func NewPGX(ctx context.Context) (*pgxpool.Pool, error) {
	creds, err := readDBCredentials()
	if err != nil {
		return nil, err
	}

	sslMode := os.Getenv("DATABASE_SSLMODE")
	if sslMode == "" {
		sslMode = "verify-full"
	}

	tlsMode := os.Getenv("DATABASE_TLS_MODE")
	if tlsMode != mTLS && tlsMode != TLS {
		tlsMode = "disable"
		sslMode = "disable"
	}

	tlsConfig, err := buildTLSConfig(tlsMode, creds.ServerFQDN)
	if err != nil {
		return nil, err
	}
	dsn := buildDSN(creds, sslMode)
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}
	if tlsConfig != nil {
		cfg.ConnConfig.TLSConfig = tlsConfig
	}

	// TODO: tune the pool
	cfg.MaxConns = 10
	cfg.MinConns = 1
	cfg.MaxConnLifetime = time.Minute * 30
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("new with config: %w", err)
	}
	return pool, nil
}
