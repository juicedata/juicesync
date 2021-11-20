module github.com/juicedata/juicesync

go 1.16

require (
	github.com/juicedata/juicefs v0.17.3-0.20211120094803-7c6f23f1f9bd
	github.com/sirupsen/logrus v1.7.0
	github.com/urfave/cli/v2 v2.3.0
	golang.org/x/term v0.0.0-20201126162022-7de9c90e9dd1
)

replace github.com/golang-jwt/jwt v3.2.2+incompatible => github.com/dgrijalva/jwt-go v3.2.0+incompatible

replace github.com/dgrijalva/jwt-go v3.2.0+incompatible => github.com/golang-jwt/jwt v3.2.2+incompatible
