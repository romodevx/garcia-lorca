module github.com/garcia-lorca/services/gateway

go 1.24

require github.com/garcia-lorca/pkg v0.0.0

require (
	connectrpc.com/connect v1.20.0 // indirect
	google.golang.org/protobuf v1.36.11 // indirect
)

replace github.com/garcia-lorca/pkg => ../../pkg
