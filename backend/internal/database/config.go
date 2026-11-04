package database

import (
	"context"
	"crypto/tls"
	"database/sql"
	"errors"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	mysql "github.com/go-sql-driver/mysql"
)

var (
	tlsRegistrationMu sync.Mutex
	registeredTLS     = make(map[string]struct{})
)

// LoadConfig reads secrets only on the server. Credentials are never copied into
// the repository, browser, command arguments, logs or status responses.
func LoadConfig() (*mysql.Config, error) {
	value := strings.TrimSpace(os.Getenv("DATABASE_URL"))
	if value == "" {
		file := os.Getenv("DATABASE_CREDENTIAL_FILE")
		if file == "" {
			root := os.Getenv("FINBOOK_ROOT")
			if root == "" {
				root = ".."
			}
			root, _ = filepath.Abs(root)
			file = filepath.Join(root, "backend", ".local", "mysql.url")
		}
		data, err := os.ReadFile(file)
		if err != nil {
			return nil, errors.New("database connection file unavailable")
		}
		value = string(data)
	}
	return ParseConfig(value)
}

func ParseConfig(value string) (*mysql.Config, error) {
	value = strings.TrimSpace(value)
	if strings.HasPrefix(value, "DATABASE_URL") {
		key, rest, ok := strings.Cut(value, "=")
		if !ok || strings.TrimSpace(key) != "DATABASE_URL" {
			return nil, errors.New("invalid database configuration")
		}
		value = strings.TrimSpace(rest)
	}
	if len(value) >= 2 && ((value[0] == '"' && value[len(value)-1] == '"') || (value[0] == '\'' && value[len(value)-1] == '\'')) {
		value = value[1 : len(value)-1]
	}
	uri, err := url.Parse(value)
	if err != nil || uri.Scheme != "mysql" || uri.Hostname() == "" || uri.User == nil || uri.Path == "" {
		return nil, errors.New("invalid MySQL database configuration")
	}
	username := uri.User.Username()
	password, _ := uri.User.Password()
	databaseName := strings.TrimPrefix(uri.Path, "/")
	if username == "" || password == "" || databaseName == "" || strings.Contains(databaseName, "/") || uri.Fragment != "" {
		return nil, errors.New("invalid MySQL database configuration")
	}

	host := uri.Hostname()
	port := uri.Port()
	if port == "" {
		port = "3306"
	}
	portNumber, err := strconv.Atoi(port)
	if err != nil || portNumber < 1 || portNumber > 65535 {
		return nil, errors.New("invalid MySQL database port")
	}
	query := uri.Query()
	tlsMode := query.Get("tls")
	if len(query["tls"]) > 1 {
		return nil, errors.New("invalid MySQL TLS setting")
	}
	query.Del("tls")
	if len(query["parseTime"]) > 1 || (query.Get("parseTime") != "" && query.Get("parseTime") != "true") {
		return nil, errors.New("MySQL parseTime must be true")
	}
	query.Del("parseTime")
	if len(query["charset"]) > 1 {
		return nil, errors.New("invalid MySQL charset setting")
	}
	if charset := query.Get("charset"); charset != "" && !strings.EqualFold(charset, "utf8mb4") {
		return nil, errors.New("MySQL charset must be utf8mb4")
	}
	query.Del("charset")
	if len(query) != 0 {
		return nil, errors.New("unsupported MySQL connection option")
	}
	config := mysql.NewConfig()
	config.User = username
	config.Passwd = password
	config.Net = "tcp"
	config.Addr = net.JoinHostPort(host, port)
	config.DBName = databaseName
	config.ParseTime = true
	config.Loc = time.UTC
	config.Timeout = 8 * time.Second
	config.ReadTimeout = 8 * time.Second
	config.WriteTimeout = 8 * time.Second
	config.Params = map[string]string{"charset": "utf8mb4"}

	localHost := strings.EqualFold(host, "localhost") || net.ParseIP(host).IsLoopback()
	switch {
	case localHost && (tlsMode == "" || tlsMode == "false"):
		config.TLSConfig = ""
	case tlsMode == "verify-full":
		name := "finbook-verify-" + host
		tlsRegistrationMu.Lock()
		if _, loaded := registeredTLS[name]; !loaded {
			tlsConfig := &tls.Config{MinVersion: tls.VersionTLS12, ServerName: host}
			if err := mysql.RegisterTLSConfig(name, tlsConfig); err != nil {
				tlsRegistrationMu.Unlock()
				return nil, errors.New("could not configure verified MySQL TLS")
			}
			registeredTLS[name] = struct{}{}
		}
		tlsRegistrationMu.Unlock()
		config.TLSConfig = name
	case localHost:
		return nil, errors.New("local MySQL must use tls=false or verified TLS")
	default:
		return nil, errors.New("remote MySQL requires tls=verify-full")
	}

	return config, nil
}

// Connect creates a bounded connection pool and verifies the configured server.
func Connect(ctx context.Context, config *mysql.Config) (*sql.DB, error) {
	if config == nil {
		return nil, errors.New("database not configured")
	}
	db, err := sql.Open("mysql", config.FormatDSN())
	if err != nil {
		return nil, errors.New("invalid MySQL database configuration")
	}
	db.SetMaxOpenConns(2)
	db.SetMaxIdleConns(2)
	db.SetConnMaxLifetime(5 * time.Minute)
	if err := db.PingContext(ctx); err != nil {
		_ = db.Close()
		return nil, errors.New("MySQL connection failed")
	}
	return db, nil
}
