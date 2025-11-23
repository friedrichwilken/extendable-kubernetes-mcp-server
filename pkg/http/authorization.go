package http

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/coreos/go-oidc/v3/oidc"
	"github.com/go-jose/go-jose/v4"
	"github.com/go-jose/go-jose/v4/jwt"
	"golang.org/x/oauth2"
	authenticationapiv1 "k8s.io/api/authentication/v1"
	"k8s.io/klog/v2"
	"k8s.io/utils/strings/slices"

	"github.com/containers/kubernetes-mcp-server/pkg/config"
	"github.com/containers/kubernetes-mcp-server/pkg/mcp"
)

type KubernetesApiTokenVerifier interface {
	// KubernetesApiVerifyToken TODO: clarify proper implementation
	KubernetesApiVerifyToken(ctx context.Context, cluster, token, audience string) (*authenticationapiv1.UserInfo, []string, error)
	// GetTargetParameterName returns the parameter name used for target identification in MCP requests
	GetTargetParameterName() string
}

// extractTargetFromRequest extracts cluster parameter from MCP request body
func extractTargetFromRequest(r *http.Request, targetName string) (string, error) {
	if r.Body == nil {
		return "", nil
	}

	// Read the body
	body, err := io.ReadAll(r.Body)
	if err != nil {
		return "", err
	}

	// Restore the body for downstream handlers
	r.Body = io.NopCloser(bytes.NewBuffer(body))

	// Parse the MCP request
	var mcpRequest struct {
		Params struct {
			Arguments map[string]interface{} `json:"arguments"`
		} `json:"params"`
	}

	if err := json.Unmarshal(body, &mcpRequest); err != nil {
		// If we can't parse the request, just return empty cluster (will use default)
		return "", nil
	}

	// Extract target parameter
	if cluster, ok := mcpRequest.Params.Arguments[targetName].(string); ok {
		return cluster, nil
	}

	return "", nil
}

// write401 sends a 401/Unauthorized response with WWW-Authenticate header.
func write401(w http.ResponseWriter, wwwAuthenticateHeader, errorType, message string) {
	w.Header().Set("WWW-Authenticate", wwwAuthenticateHeader+fmt.Sprintf(`, error=%q`, errorType))
	http.Error(w, message, http.StatusUnauthorized)
}

// validateToken performs all token validation steps: offline, OIDC provider, and Kubernetes API
func validateToken(
	ctx context.Context,
	token string,
	staticConfig *config.StaticConfig,
	oidcProvider *oidc.Provider,
	verifier KubernetesApiTokenVerifier,
	r *http.Request,
	httpClient *http.Client,
) (*JWTClaims, error) {
	// Parse JWT claims
	claims, err := ParseJWTClaims(token)
	if err == nil && claims == nil {
		err = fmt.Errorf("failed to parse JWT claims from token")
	}

	// Offline validation
	if err == nil {
		err = claims.ValidateOffline(staticConfig.OAuthAudience)
	}

	// Online OIDC provider validation
	if err == nil {
		err = claims.ValidateWithProvider(ctx, staticConfig.OAuthAudience, oidcProvider)
	}

	// Scopes propagation
	if err == nil {
		scopes := claims.GetScopes()
		klog.V(2).Infof("JWT token validated - Scopes: %v", scopes)
		r = r.WithContext(context.WithValue(r.Context(), mcp.TokenScopesContextKey, scopes))
	}

	// Token exchange with OIDC provider
	if err == nil {
		exchangedToken, exchangeErr := exchangeTokenIfNeeded(ctx, claims, staticConfig, oidcProvider, httpClient, r)
		if exchangeErr != nil {
			err = exchangeErr
		} else if exchangedToken != "" {
			claims, err = ParseJWTClaims(exchangedToken)
		}
	}

	// Kubernetes API Server TokenReview validation
	if err == nil && staticConfig.ValidateToken {
		err = validateTokenWithKubernetes(ctx, r, verifier, claims, staticConfig.OAuthAudience)
	}

	return claims, err
}

// exchangeTokenIfNeeded performs token exchange if STS is enabled
func exchangeTokenIfNeeded(
	ctx context.Context,
	claims *JWTClaims,
	staticConfig *config.StaticConfig,
	oidcProvider *oidc.Provider,
	httpClient *http.Client,
	r *http.Request,
) (string, error) {
	sts := NewFromConfig(staticConfig, oidcProvider)
	if !sts.IsEnabled() {
		return "", nil
	}

	if httpClient != nil {
		ctx = context.WithValue(ctx, oauth2.HTTPClient, httpClient)
	}

	exchangedToken, err := sts.ExternalAccountTokenExchange(ctx, &oauth2.Token{
		AccessToken: claims.Token,
		TokenType:   "Bearer",
	})
	if err != nil {
		return "", err
	}

	r.Header.Set("Authorization", fmt.Sprintf("Bearer %s", exchangedToken.AccessToken))
	return exchangedToken.AccessToken, nil
}

// validateTokenWithKubernetes validates token against Kubernetes API Server
func validateTokenWithKubernetes(ctx context.Context, r *http.Request, verifier KubernetesApiTokenVerifier, claims *JWTClaims, audience string) error {
	targetParameterName := verifier.GetTargetParameterName()
	cluster, clusterErr := extractTargetFromRequest(r, targetParameterName)
	if clusterErr != nil {
		klog.V(2).Infof("Failed to extract cluster from request, using default: %v", clusterErr)
	}
	return claims.ValidateWithKubernetesApi(ctx, audience, cluster, verifier)
}

// AuthorizationMiddleware validates the OAuth flow for protected resources.
//
// The flow is skipped for unprotected resources, such as health checks and well-known endpoints.
//
//	There are several auth scenarios supported by this middleware:
//
//	 1. requireOAuth is false:
//
//	    - The OAuth flow is skipped, and the server is effectively unprotected.
//	    - The request is passed to the next handler without any validation.
//
//	    see TestAuthorizationRequireOAuthFalse
//
//	 2. requireOAuth is set to true, server is protected:
//
//	    2.1. Raw Token Validation (oidcProvider is nil):
//	         - The token is validated offline for basic sanity checks (expiration).
//	         - If OAuthAudience is set, the token is validated against the audience.
//	         - If ValidateToken is set, the token is then used against the Kubernetes API Server for TokenReview.
//
//	         see TestAuthorizationRawToken
//
//	    2.2. OIDC Provider Validation (oidcProvider is not nil):
//	         - The token is validated offline for basic sanity checks (audience and expiration).
//	         - If OAuthAudience is set, the token is validated against the audience.
//	         - The token is then validated against the OIDC Provider.
//	         - If ValidateToken is set, the token is then used against the Kubernetes API Server for TokenReview.
//
//	         see TestAuthorizationOidcToken
//
//	    2.3. OIDC Token Exchange (oidcProvider is not nil, StsClientId and StsAudience are set):
//	         - The token is validated offline for basic sanity checks (audience and expiration).
//	         - If OAuthAudience is set, the token is validated against the audience.
//	         - The token is then validated against the OIDC Provider.
//	         - If the token is valid, an external account token exchange is performed using
//	           the OIDC Provider to obtain a new token with the specified audience and scopes.
//	         - If ValidateToken is set, the exchanged token is then used against the Kubernetes API Server for TokenReview.
//
//	         see TestAuthorizationOidcTokenExchange
func AuthorizationMiddleware(
	staticConfig *config.StaticConfig,
	oidcProvider *oidc.Provider,
	verifier KubernetesApiTokenVerifier,
	httpClient *http.Client,
) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == healthEndpoint || slices.Contains(WellKnownEndpoints, r.URL.EscapedPath()) {
				next.ServeHTTP(w, r)
				return
			}
			if !staticConfig.RequireOAuth {
				next.ServeHTTP(w, r)
				return
			}

			wwwAuthenticateHeader := "Bearer realm=\"Kubernetes MCP Server\""
			if staticConfig.OAuthAudience != "" {
				wwwAuthenticateHeader += fmt.Sprintf(`, audience=%q`, staticConfig.OAuthAudience)
			}

			authHeader := r.Header.Get("Authorization")
			if authHeader == "" || !strings.HasPrefix(authHeader, "Bearer ") {
				klog.V(1).Infof("Authentication failed - missing or invalid bearer token: %s %s from %s", r.Method, r.URL.Path, r.RemoteAddr)
				write401(w, wwwAuthenticateHeader, "missing_token", "Unauthorized: Bearer token required")
				return
			}

			token := strings.TrimPrefix(authHeader, "Bearer ")

			_, err := validateToken(r.Context(), token, staticConfig, oidcProvider, verifier, r, httpClient)
			if err != nil {
				klog.V(1).Infof("Authentication failed - JWT validation error: %s %s from %s, error: %v", r.Method, r.URL.Path, r.RemoteAddr, err)
				write401(w, wwwAuthenticateHeader, "invalid_token", "Unauthorized: Invalid token")
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

var allSignatureAlgorithms = []jose.SignatureAlgorithm{
	jose.EdDSA,
	jose.HS256,
	jose.HS384,
	jose.HS512,
	jose.RS256,
	jose.RS384,
	jose.RS512,
	jose.ES256,
	jose.ES384,
	jose.ES512,
	jose.PS256,
	jose.PS384,
	jose.PS512,
}

type JWTClaims struct {
	jwt.Claims
	Token string `json:"-"`
	Scope string `json:"scope,omitempty"`
}

func (c *JWTClaims) GetScopes() []string {
	if c.Scope == "" {
		return nil
	}
	return strings.Fields(c.Scope)
}

// ValidateOffline Checks if the JWT claims are valid and if the audience matches the expected one.
func (c *JWTClaims) ValidateOffline(audience string) error {
	expected := jwt.Expected{}
	if audience != "" {
		expected.AnyAudience = jwt.Audience{audience}
	}
	if err := c.Validate(expected); err != nil {
		return fmt.Errorf("JWT token validation error: %v", err)
	}
	return nil
}

// ValidateWithProvider validates the JWT claims against the OIDC provider.
func (c *JWTClaims) ValidateWithProvider(ctx context.Context, audience string, provider *oidc.Provider) error {
	if provider != nil {
		verifier := provider.Verifier(&oidc.Config{
			ClientID: audience,
		})
		_, err := verifier.Verify(ctx, c.Token)
		if err != nil {
			return fmt.Errorf("OIDC token validation error: %v", err)
		}
	}
	return nil
}

func (c *JWTClaims) ValidateWithKubernetesApi(ctx context.Context, audience, cluster string, verifier KubernetesApiTokenVerifier) error {
	if verifier != nil {
		_, _, err := verifier.KubernetesApiVerifyToken(ctx, cluster, c.Token, audience)
		if err != nil {
			return fmt.Errorf("kubernetes API token validation error: %v", err)
		}
	}
	return nil
}

func ParseJWTClaims(token string) (*JWTClaims, error) {
	tkn, err := jwt.ParseSigned(token, allSignatureAlgorithms)
	if err != nil {
		return nil, fmt.Errorf("failed to parse JWT token: %w", err)
	}
	claims := &JWTClaims{}
	err = tkn.UnsafeClaimsWithoutVerification(claims)
	claims.Token = token
	return claims, err
}
