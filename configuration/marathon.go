package configuration

import (
	"strings"
)

/*
	Mesos Marathon configuration
*/
type Marathon struct {
	// comma separated marathon http endpoints including port number
	Endpoint string
	// marathon http basic auth
	BasicAuth string
}

func (m Marathon) Endpoints() []string {
	return strings.Split(m.Endpoint, ",")
}
