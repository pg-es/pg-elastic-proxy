package pg

import (
	"net/url"
)

// ConnString builds a postgres:// URL from a host:port hostPort and the
// given user, password, and database name.
func ConnString(hostPort, user, password, dbname string) string {
	connURL := url.URL{
		Scheme: "postgres",
		User:   url.UserPassword(user, password),
		Host:   hostPort,
		Path:   "/" + dbname,
	}

	return connURL.String()
}
