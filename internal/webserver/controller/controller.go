package controller

import "fmt"

const (
	defaultHttpPort  = 80
	defaultHttpsPort = 443
)

func UrlPort(protocol string, port int) string {
	urlPort := fmt.Sprintf(":%d", port)
	if (port == defaultHttpPort && protocol == "http") ||
		(port == defaultHttpsPort && protocol == "https") {
		urlPort = ""
	}
	return urlPort
}
