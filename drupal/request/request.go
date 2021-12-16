package request

import "github.com/cristalhq/jwt/v4"

type Context struct {
	token   *jwt.Token
	headers map[string]string
}

func New() *Context {
	return &Context{}
}
func (rc *Context) WithHeader(name, value string) *Context {
	if rc.headers == nil {
		rc.headers = make(map[string]string)
	}
	rc.headers[name] = value
	return rc
}

func (rc *Context) WithToken(token *jwt.Token) *Context {
	rc.token = token
	return rc
}

func (rc *Context) Headers() map[string]string {
	if rc.headers == nil {
		return make(map[string]string, 1)
	}

	return rc.headers
}

func (rc *Context) Token() *jwt.Token {
	return rc.token
}
