package main

import (
	"github.com/Kong/go-pdk/server"
)

const Version = "0.1.0"
const Priority = 1000

func main() {
	server.StartServer(New, Version, Priority)
}
