module github.com/garcia-lorca/services/iam

go 1.24

require (
	github.com/garcia-lorca/pkg v0.0.0
	github.com/golang-jwt/jwt/v5 v5.3.1
	golang.org/x/crypto v0.53.0
)

replace github.com/garcia-lorca/pkg => ../../pkg
