package api

import (
	"net/http"

	"github.com/DevopsArtFactory/tenet/internal/requestactor"
)

func requestActorMiddleware(trustAuthenticatedHeaders bool, authSession authSessionConfig, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := requestactor.WithContext(r.Context(), requestactor.FromHeaders(r.Header))
		if trustAuthenticatedHeaders {
			identity := requestactor.AuthenticatedIdentityFromHeaders(r.Header)
			ctx = requestactor.WithAuthenticatedIdentityContext(ctx, identity)
			ctx = requestactor.WithAuthenticatedIdentityProvenanceContext(ctx, requestactor.AuthenticatedIdentityProvenanceFromHeaders(r.Header))
			ctx = requestactor.WithAuthenticatedRolesContext(ctx, requestactor.AuthenticatedRolesFromHeaders(r.Header))
			ctx = requestactor.WithAuthenticatedGroupsContext(ctx, requestactor.AuthenticatedGroupsFromHeaders(r.Header))
			ctx = requestactor.WithAuthorizedDeploymentsContext(ctx, requestactor.AuthorizedDeploymentsFromHeaders(r.Header))
			ctx = requestactor.WithAuthorizedProjectEnvironmentScopesContext(ctx, requestactor.AuthorizedProjectEnvironmentScopesFromHeaders(r.Header))
			if !identity.Authenticated {
				ctx = contextWithAuthSession(ctx, r, authSession)
			}
		} else {
			ctx = contextWithAuthSession(ctx, r, authSession)
		}
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}
