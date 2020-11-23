package csrf

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"fmt"
	"html/template"
	"net/url"

	"github.com/kataras/iris/v12"
)

// Token returns a masked CSRF token ready for passing into HTML template or
// a JSON response body. An empty token will be returned if the middleware
// has not been applied (which will fail subsequent validation).
func Token(ctx iris.Context) string {
	return ctx.Values().GetString(tokenKey)
}

// FailureReason makes CSRF validation errors available in the request context.
// This is useful when you want to log the cause of the error or report it to
// client.
func FailureReason(ctx iris.Context) error {
	return ctx.GetErr()
}

// UnsafeSkipCheck will skip the CSRF check for any requests.  This must be
// called before the CSRF middleware.
//
// Note: You should not set this without otherwise securing the request from
// CSRF attacks. The primary use-case for this function is to turn off CSRF
// checks for non-browser clients using authorization tokens against your API.
func UnsafeSkipCheck(ctx iris.Context) {
	ctx.Values().Set(skipCheckKey, true)
}

// TemplateField is a template helper for html/template that provides an <input> field
// populated with a CSRF token.
//
// Example:
//
//      // The following tag in our form.tmpl template:
//      {{ .csrfField }}
//
//      // ... becomes:
//      <input type="hidden" name="csrf.token" value="<token>">
//
func TemplateField(ctx iris.Context) template.HTML {
	name := ctx.Values().GetString(formKey)
	if name == "" {
		return template.HTML("")
	}

	fragment := fmt.Sprintf(`<input type="hidden" name="%s" value="%s">`, name, Token(ctx))
	return template.HTML(fragment)
}

// mask returns a unique-per-request token to mitigate the BREACH attack
// as per http://breachattack.com/#mitigations
//
// The token is generated by XOR'ing a one-time-pad and the base (session) CSRF
// token and returning them together as a 64-byte slice. This effectively
// randomises the token on a per-request basis without breaking multiple browser
// tabs/windows.
func mask(realToken []byte) string {
	otp, err := generateRandomBytes(tokenLength)
	if err != nil {
		return ""
	}

	// XOR the OTP with the real token to generate a masked token. Append the
	// OTP to the front of the masked token to allow unmasking in the subsequent
	// request.
	return base64.StdEncoding.EncodeToString(append(otp, xorToken(otp, realToken)...))
}

// unmask splits the issued token (one-time-pad + masked token) and returns the
// unmasked request token for comparison.
func unmask(issued []byte) []byte {
	// Issued tokens are always masked and combined with the pad.
	if len(issued) != tokenLength*2 {
		return nil
	}

	// We now know the length of the byte slice.
	otp := issued[tokenLength:]
	masked := issued[:tokenLength]

	// Unmask the token by XOR'ing it against the OTP used to mask it.
	return xorToken(otp, masked)
}

// generateRandomBytes returns securely generated random bytes.
// It will return an error if the system's secure random number generator
// fails to function correctly.
func generateRandomBytes(n int) ([]byte, error) {
	b := make([]byte, n)
	_, err := rand.Read(b)
	// err == nil only if len(b) == n
	if err != nil {
		return nil, err
	}

	return b, nil

}

// sameOrigin returns true if URLs a and b share the same origin. The same
// origin is defined as host (which includes the port) and scheme.
func sameOrigin(a, b *url.URL) bool {
	return (a.Scheme == b.Scheme && a.Host == b.Host)
}

// compare securely (constant-time) compares the unmasked token from the request
// against the real token from the session.
func compareTokens(a, b []byte) bool {
	return subtle.ConstantTimeCompare(a, b) == 1
}

// xorToken XORs tokens ([]byte) to provide unique-per-request CSRF tokens. It
// will return a masked token if the base token is XOR'ed with a one-time-pad.
// An unmasked token will be returned if a masked token is XOR'ed with the
// one-time-pad used to mask it.
func xorToken(a, b []byte) []byte {
	n := len(a)
	if len(b) < n {
		n = len(b)
	}

	res := make([]byte, n)

	for i := 0; i < n; i++ {
		res[i] = a[i] ^ b[i]
	}

	return res
}

// contains is a helper function to check if a string exists in a slice - e.g.
// whether a HTTP method exists in a list of safe methods.
func contains(vals []string, s string) bool {
	for _, v := range vals {
		if v == s {
			return true
		}
	}

	return false
}

// envError stores a CSRF error in the request context.
func envError(ctx iris.Context, err error) {
	ctx.SetErr(err)
}
