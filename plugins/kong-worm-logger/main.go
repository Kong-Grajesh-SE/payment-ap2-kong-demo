package main

import (
	"github.com/Kong/go-pdk/server"
)

const Version = "0.1.0"
const Priority = 100

func main() {
	server.StartServer(New, Version, Priority)
}
