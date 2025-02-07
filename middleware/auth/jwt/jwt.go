package jwt

import (
	"context"
	"fmt"
	"strings"

	"github.com/golang-jwt/jwt/v4"

	"github.com/go-kratos/kratos/v2/errors"
	"github.com/go-kratos/kratos/v2/middleware"
	"github.com/go-kratos/kratos/v2/transport"
)

type authKey struct{}

const (

	// bearerWord the bearer key word for authorization
	bearerWord string = "Bearer"

	// bearerFormat authorization token format
	bearerFormat string = "Bearer %s"

	// authorizationKey holds the key used to store the JWT Token in the request tokenHeader.
	authorizationKey string = "Authorization"

	// reason holds the error reason.
	reason string = "UNAUTHORIZED"
)

var (
	ErrMissingJwtToken        = errors.Unauthorized(reason, "JWT token is missing")
	ErrMissingKeyFunc         = errors.Unauthorized(reason, "keyFunc is missing")
	ErrTokenInvalid           = errors.Unauthorized(reason, "Token is invalid")
	ErrTokenExpired           = errors.Unauthorized(reason, "JWT token has expired")
	ErrTokenParseFail         = errors.Unauthorized(reason, "Fail to parse JWT token ")
	ErrUnSupportSigningMethod = errors.Unauthorized(reason, "Wrong signing method")
	ErrWrongContext           = errors.Unauthorized(reason, "Wrong context for middleware")
	ErrNeedTokenProvider      = errors.Unauthorized(reason, "Token provider is missing")
	ErrSignToken              = errors.Unauthorized(reason, "Can not sign token.Is the key correct?")
	ErrGetKey                 = errors.Unauthorized(reason, "Can not get key while signing token")
)

// Option is jwt option.
type Option func(*options)

// Parser is a jwt parser
type options struct {
	signingMethod jwt.SigningMethod
	claims        jwt.Claims
	tokenHeader   map[string]interface{}
}

// WithSigningMethod with signing method option.
func WithSigningMethod(method jwt.SigningMethod) Option {
	return func(o *options) {
		o.signingMethod = method
	}
}

// WithClaims with customer claim
func WithClaims(claims jwt.Claims) Option {
	return func(o *options) {
		o.claims = claims
	}
}

// WithTokenHeader withe customer tokenHeader for client side
func WithTokenHeader(header map[string]interface{}) Option {
	return func(o *options) {
		o.tokenHeader = header
	}
}

// Server is a server auth middleware. Check the token and extract the info from token.
func Server(keyFunc jwt.Keyfunc, opts ...Option) middleware.Middleware {
	o := &options{
		signingMethod: jwt.SigningMethodHS256,
		claims:        jwt.StandardClaims{},
	}
	for _, opt := range opts {
		opt(o)
	}
	return func(handler middleware.Handler) middleware.Handler {
		return func(ctx context.Context, req interface{}) (interface{}, error) {
			if header, ok := transport.FromServerContext(ctx); ok {
				if keyFunc == nil {
					return nil, ErrMissingKeyFunc
				}
				auths := strings.SplitN(header.RequestHeader().Get(authorizationKey), " ", 2)
				if len(auths) != 2 || !strings.EqualFold(auths[0], bearerWord) {
					return nil, ErrMissingJwtToken
				}
				jwtToken := auths[1]
				tokenInfo, err := jwt.ParseWithClaims(jwtToken, o.claims, keyFunc)
				if err != nil {
					if ve, ok := err.(*jwt.ValidationError); ok {
						if ve.Errors&jwt.ValidationErrorMalformed != 0 {
							return nil, ErrTokenInvalid
						} else if ve.Errors&(jwt.ValidationErrorExpired|jwt.ValidationErrorNotValidYet) != 0 {
							return nil, ErrTokenExpired
						} else {
							return nil, ErrTokenParseFail
						}
					}
					return nil, errors.Unauthorized(reason, err.Error())
				} else if !tokenInfo.Valid {
					return nil, ErrTokenInvalid
				} else if tokenInfo.Method != o.signingMethod {
					return nil, ErrUnSupportSigningMethod
				}
				ctx = NewContext(ctx, tokenInfo.Claims)
				return handler(ctx, req)
			}
			return nil, ErrWrongContext
		}
	}
}

// Client is a client jwt middleware.
func Client(keyProvider jwt.Keyfunc, opts ...Option) middleware.Middleware {
	o := &options{
		signingMethod: jwt.SigningMethodHS256,
		claims:        jwt.StandardClaims{},
	}
	for _, opt := range opts {
		opt(o)
	}
	return func(handler middleware.Handler) middleware.Handler {
		return func(ctx context.Context, req interface{}) (interface{}, error) {
			if keyProvider == nil {
				return nil, ErrNeedTokenProvider
			}
			token := jwt.NewWithClaims(o.signingMethod, o.claims)
			if o.tokenHeader != nil {
				for k, v := range o.tokenHeader {
					token.Header[k] = v
				}
			}
			key, err := keyProvider(token)
			if err != nil {
				return nil, ErrGetKey
			}
			tokenStr, err := token.SignedString(key)
			if err != nil {
				return nil, ErrSignToken
			}
			if clientContext, ok := transport.FromClientContext(ctx); ok {
				clientContext.RequestHeader().Set(authorizationKey, fmt.Sprintf(bearerFormat, tokenStr))
				return handler(ctx, req)
			}
			return nil, ErrWrongContext
		}
	}
}

// NewContext put auth info into context
func NewContext(ctx context.Context, info jwt.Claims) context.Context {
	return context.WithValue(ctx, authKey{}, info)
}

// FromContext extract auth info from context
func FromContext(ctx context.Context) (token jwt.Claims, ok bool) {
	token, ok = ctx.Value(authKey{}).(jwt.Claims)
	return
}
