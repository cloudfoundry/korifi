package apis

import (
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"net/http"
	"regexp"
	"time"

	"code.cloudfoundry.org/cf-k8s-controllers/api/authorization"
	"github.com/go-http-utils/headers"
	"github.com/go-logr/logr"
)

type AuthenticationMiddleware struct {
	logger                   logr.Logger
	authInfoParser           AuthInfoParser
	identityProvider         IdentityProvider
	unauthenticatedEndpoints map[string]interface{}
}

//counterfeiter:generate -o fake -fake-name AuthInfoParser . AuthInfoParser

type AuthInfoParser interface {
	Parse(authHeader string) (authorization.Info, error)
}

func NewAuthenticationMiddleware(logger logr.Logger, authInfoParser AuthInfoParser, identityProvider IdentityProvider) *AuthenticationMiddleware {
	return &AuthenticationMiddleware{
		logger:           logger,
		authInfoParser:   authInfoParser,
		identityProvider: identityProvider,
		unauthenticatedEndpoints: map[string]interface{}{
			"/":            struct{}{},
			"/v3":          struct{}{},
			"/api/v1/info": struct{}{},
		},
	}
}

func (a *AuthenticationMiddleware) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if a.isUnauthenticatedEndpoint(r.URL.Path) {
			next.ServeHTTP(w, r)
			return
		}

		var authInfo authorization.Info
		var err error

		if len(r.TLS.PeerCertificates) > 0 {
			clientCert := r.TLS.PeerCertificates[0]

			caCertBlock, _ := pem.Decode([]byte(`
-----BEGIN CERTIFICATE-----
MIIC/jCCAeagAwIBAgIBADANBgkqhkiG9w0BAQsFADAVMRMwEQYDVQQDEwprdWJl
cm5ldGVzMB4XDTIyMDMwNDEwNTk0MFoXDTMyMDMwMTEwNTk0MFowFTETMBEGA1UE
AxMKa3ViZXJuZXRlczCCASIwDQYJKoZIhvcNAQEBBQADggEPADCCAQoCggEBAPUJ
K2a34J9ev1aUxWOoOP5pEwUQZuaFGOAi/0MpZzZM3fW8J0ynkSoE+Toqr7J51yYm
kzpcAy1hoyqN6v0e6j20EG+QrNeA3aB7/PrTMwGlcv1EVJTj+y4ahpwqAFrEPJw2
n2U07Lf95xUBeFbSFc9GldPqroXdRQY1T8/mmlhhBFft99E5aRW00tyFEWQrauPg
g2AxbsCWvuh2JsC3dTMWCKSywzS69sy3TVmpyOV83cXxkyG6dusXLIVaFVgOzsEC
G3LVjDfAUMBZCbyw6k6S1n93cehyeCVZdioM83fFDD73MSL4IoGViUHHyk3ipfbq
Cv5OZjVR/xLt6TsVhqsCAwEAAaNZMFcwDgYDVR0PAQH/BAQDAgKkMA8GA1UdEwEB
/wQFMAMBAf8wHQYDVR0OBBYEFGhtOxoGDO9Fm7IgT5iITr9fFMCdMBUGA1UdEQQO
MAyCCmt1YmVybmV0ZXMwDQYJKoZIhvcNAQELBQADggEBABVRCfDzJ/Vv3iM3YcYA
5bsqvR33q84ZHsZfMu2x4+O7AmNtCJN0HoDRF3/7llugHVL1TO6yGNqDTRKTccfw
qsIz88ScytPjZtQ2LNWQuCmXp39/tvADX1JL1/C7Sev5i/CzBOGWraaoUDm3+Zv5
4NDZcafpWoZz1IKhhCJKGnc6L9LNWYZePcj0vC/benjKowMlmxPyFhNYadB6caP9
oi90l99JwQbIO/nOXvQFtHERdEEIehNr7GGoc8LzU82Wq/WH8CSPl8Phui7BAErX
jrvLY7adNgPXzV2TgP1TE+lf2B+k/Vjimk09UQh78H099Ex7xly5SYmv4Ct0vdQZ
0rI=
-----END CERTIFICATE-----
`))
			caCert, err := x509.ParseCertificate(caCertBlock.Bytes)
			if err != nil {
				a.logger.Error(err, "failed to parse ca cert")
				writeInvalidAuthErrorResponse(w)
				return

			}

			if err := clientCert.CheckSignatureFrom(caCert); err != nil {
				a.logger.Info("failed to validate client cert against CA", "error", err)
				writeInvalidAuthErrorResponse(w)
				return
			}

			if time.Now().Before(clientCert.NotBefore) {
				a.logger.Info("certificate not valid yet")
				writeInvalidAuthErrorResponse(w)
				return
			}

			if time.Now().After(clientCert.NotAfter) {
				a.logger.Info("certificate expired")
				writeInvalidAuthErrorResponse(w)
				return
			}

			fmt.Printf("About to use %s as user name\n", clientCert.Subject.CommonName)

			authInfo = authorization.Info{Username: clientCert.Subject.CommonName}
		} else {
			authInfo, err = a.authInfoParser.Parse(r.Header.Get(headers.Authorization))
			if err != nil {
				if authorization.IsNotAuthenticated(err) {
					writeNotAuthenticatedErrorResponse(w)
					return
				}

				if authorization.IsInvalidAuth(err) {
					writeInvalidAuthErrorResponse(w)
					return
				}

				a.logger.Error(err, "failed to parse auth info")
				writeUnknownErrorResponse(w)
				return
			}
		}

		r = r.WithContext(authorization.NewContext(r.Context(), &authInfo))

		_, err = a.identityProvider.GetIdentity(r.Context(), authInfo)
		if err != nil {
			if authorization.IsInvalidAuth(err) {
				writeInvalidAuthErrorResponse(w)
				return
			}

			a.logger.Error(err, "failed to get identity")
			writeUnknownErrorResponse(w)
			return
		}

		next.ServeHTTP(w, r)
	})
}

// Eventually we will want to authenticate the log-cache read endpoint and go back to using the simple
// unauthenticatedEndpoints map. For now though we need to do a more complicated regexp match against
// the path since the log-cache read endpoint includes an arbitrary guid as part of its path
var logCacheReadEndpointRegexp = regexp.MustCompile(`\/api\/v1\/read\/[0-9a-fA-F\-]*$`)

func (a *AuthenticationMiddleware) isUnauthenticatedEndpoint(p string) bool {
	_, authNotRequired := a.unauthenticatedEndpoints[p]

	return authNotRequired || logCacheReadEndpointRegexp.MatchString(p)
}
