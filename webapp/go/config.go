package isureal

import (
	"net"
	"net/url"
	"os"
)

type Config struct {
	Region     string
	DBHost     string
	DBPort     string
	DBUser     string
	DBPassword string
	DBName     string
	LogLevel   string
	// DB まで遠い台（europe1・america1）で、文の準備の往復を省くときに exec を入れる（server.go）
	DBExecMode string
	// DB まで遠い台（europe1・america1）で、TLS を張らないときに disable を入れる。TLS は新しい接続のたびに往復を約 2 回足していた
	DBSSLMode string
	// アプリの問い合わせだけを asia2 の pgbouncer に向ける。ISUREAL_DB_HOST・PORT は初期化の init.sh が直接つなぐのに残す
	// （reset.sql はセッション単位の SET を使うので、transaction モードの pgbouncer を通せない）
	DBPoolHost string
	DBPoolPort string
	// asia1 だけ、読みだけのハンドラーを同じ台の同期の複製に回す（asia2 の DB の CPU が全体の天井だった）。
	// asia2 のコミットは複製が反映するまで待つ（remote_apply）ので、どの台で書いても直後の読みに結果が見える
	DBReadHost string
	DBReadPort string
}

func ConfigFromEnv() *Config {
	return &Config{
		Region:     envOrDefault("ISUREAL_REGION", "asia"),
		DBHost:     envOrDefault("ISUREAL_DB_HOST", "127.0.0.1"),
		DBPort:     envOrDefault("ISUREAL_DB_PORT", "5432"),
		DBUser:     envOrDefault("ISUREAL_DB_USER", "isureal"),
		DBPassword: envOrDefault("ISUREAL_DB_PASSWORD", "isureal"),
		DBName:     envOrDefault("ISUREAL_DB_NAME", "isureal"),
		LogLevel:   envOrDefault("ISUREAL_LOG_LEVEL", "debug"),
		DBExecMode: envOrDefault("ISUREAL_DB_EXEC_MODE", ""),
		DBSSLMode:  envOrDefault("ISUREAL_DB_SSLMODE", ""),
		DBPoolHost: envOrDefault("ISUREAL_DB_POOL_HOST", ""),
		DBPoolPort: envOrDefault("ISUREAL_DB_POOL_PORT", ""),
		DBReadHost: envOrDefault("ISUREAL_DB_READ_HOST", ""),
		DBReadPort: envOrDefault("ISUREAL_DB_READ_PORT", "5432"),
	}
}

// 複製は同じ台にあるので、TLS も文の準備の省略も要らない
func (c *Config) DatabaseReadDSN() string {
	u := url.URL{
		Scheme:   "postgres",
		User:     url.UserPassword(c.DBUser, c.DBPassword),
		Host:     net.JoinHostPort(c.DBReadHost, c.DBReadPort),
		Path:     "/" + c.DBName,
		RawQuery: "sslmode=disable",
	}
	return u.String()
}

func (c *Config) DatabaseDSN() string {
	host, port := c.DBHost, c.DBPort
	if c.DBPoolHost != "" {
		host, port = c.DBPoolHost, c.DBPoolPort
	}
	u := url.URL{
		Scheme: "postgres",
		User:   url.UserPassword(c.DBUser, c.DBPassword),
		Host:   net.JoinHostPort(host, port),
		Path:   "/" + c.DBName,
	}
	q := url.Values{}
	if c.DBExecMode != "" {
		q.Set("default_query_exec_mode", c.DBExecMode)
	}
	if c.DBSSLMode != "" {
		q.Set("sslmode", c.DBSSLMode)
	}
	u.RawQuery = q.Encode()
	return u.String()
}

func envOrDefault(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}
