package db

import (
	mysqldriver "github.com/go-sql-driver/mysql"
	"testing"
	"time"
)

func TestDSNPreservesCredentialsAndTimeParsing(t *testing.T) {
	original := Dbconfig{Host: "::1", Port: "3306", User: "feedvid", Password: "special@:/?#password", DbName: "feedsystem"}
	cfg, err := mysqldriver.ParseDSN(buildDSN(original))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Passwd != original.Password || cfg.Addr != "[::1]:3306" || cfg.DBName != original.DbName || !cfg.ParseTime || cfg.Loc != time.UTC {
		t.Fatal("DSN lost credentials, IPv6 address or time settings")
	}
	if _, exists := cfg.Params["Time"]; exists {
		t.Fatal("unknown Time server parameter")
	}
}
