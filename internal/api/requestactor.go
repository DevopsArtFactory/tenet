package api

import (
	"net/http"

	"github.com/DevopsArtFactory/tenet/internal/requestactor"
)

func requestActorMiddleware(trustAuthenticatedHeaders bool, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := requestactor.WithContext(r.Context(), requestactor.FromHeaders(r.Header))
		if trustAuthenticatedHeaders {
			ctx = requestactor.WithAuthenticatedIdentityContext(ctx, requestactor.AuthenticatedIdentityFromHeaders(r.Header))
			ctx = requestactor.WithAuthorizedDeploymentsContext(ctx, requestactor.AuthorizedDeploymentsFromHeaders(r.Header))
		}
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}
