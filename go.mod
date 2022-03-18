module github.com/juicedata/juicesync

go 1.16

require (
	github.com/juicedata/juicefs v1.0.0-beta2
	github.com/sirupsen/logrus v1.7.0
	github.com/urfave/cli/v2 v2.3.0
	golang.org/x/term v0.0.0-20201126162022-7de9c90e9dd1
)

replace github.com/dgrijalva/jwt-go v3.2.0+incompatible => github.com/golang-jwt/jwt v3.2.1+incompatible

replace github.com/vbauerster/mpb/v7 v7.0.3 => github.com/juicedata/mpb/v7 v7.0.4-0.20220216145631-6e0757f14703

