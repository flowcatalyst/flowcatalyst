package tech.flowcatalyst.platform.authentication;

import io.quarkus.arc.Arc;
import io.quarkus.arc.ManagedContext;
import io.quarkus.security.identity.AuthenticationRequestContext;
import io.quarkus.security.identity.IdentityProvider;
import io.quarkus.security.identity.SecurityIdentity;
import io.quarkus.security.identity.request.TokenAuthenticationRequest;
import io.quarkus.security.runtime.QuarkusPrincipal;
import io.quarkus.security.runtime.QuarkusSecurityIdentity;
import io.smallrye.jwt.auth.principal.ParseException;
import io.smallrye.mutiny.Uni;
import jakarta.annotation.Priority;
import jakarta.enterprise.context.ApplicationScoped;
import jakarta.inject.Inject;
import org.eclipse.microprofile.jwt.JsonWebToken;
import org.jboss.logging.Logger;
import tech.flowcatalyst.platform.principal.Principal;
import tech.flowcatalyst.platform.principal.PrincipalRepository;

import java.util.HashSet;
import java.util.Optional;
import java.util.Set;

/**
 * Identity provider that validates internal JWT tokens (from session cookies or Authorization header).
 * This enables Quarkus security to work with our internally-issued tokens.
 *
 * Uses JwtKeyService directly to verify tokens against our auto-generated or configured keys,
 * rather than relying on SmallRye JWT's default configuration which requires a JWKS URL.
 */
@ApplicationScoped
@Priority(1)
public class InternalIdentityProvider implements IdentityProvider<TokenAuthenticationRequest> {

    private static final Logger LOG = Logger.getLogger(InternalIdentityProvider.class);

    @Inject
    JwtKeyService jwtKeyService;

    @Inject
    PrincipalRepository principalRepository;

    @Override
    public Class<TokenAuthenticationRequest> getRequestType() {
        return TokenAuthenticationRequest.class;
    }

    @Override
    public Uni<SecurityIdentity> authenticate(TokenAuthenticationRequest request, AuthenticationRequestContext context) {
        return context.runBlocking(() -> {
            try {
                var token = request.getToken().getToken();

                // Resolve verification key based on token's kid header (supports key rotation)
                var kid = JwtKeyService.extractKidFromHeader(token);
                var verificationKey = jwtKeyService.getVerificationKey(kid);
                var parser = new io.smallrye.jwt.auth.principal.DefaultJWTParser();
                var jwt = parser.verify(token, verificationKey);

                // Validate issuer
                if (!jwtKeyService.getIssuer().equals(jwt.getIssuer())) {
                    LOG.debugf("Token issuer mismatch: expected %s, got %s", jwtKeyService.getIssuer(), jwt.getIssuer());
                    return null;
                }

                String principalId = jwt.getSubject();
                Set<String> roles = new HashSet<>(jwt.getGroups());

                // If no roles in token, load from embedded Principal.roles
                if (roles.isEmpty()) {
                    // Activate request context for database access
                    ManagedContext requestContext = Arc.container().requestContext();
                    boolean wasActive = requestContext.isActive();
                    if (!wasActive) {
                        requestContext.activate();
                    }
                    try {
                        Optional<Principal> principalOpt = principalRepository.findByIdOptional(principalId);
                        if (principalOpt.isPresent()) {
                            roles.addAll(principalOpt.get().getRoleNames());
                        }
                    } finally {
                        if (!wasActive) {
                            requestContext.terminate();
                        }
                    }
                }

                return QuarkusSecurityIdentity.builder()
                        .setPrincipal(new QuarkusPrincipal(principalId))
                        .addRoles(roles)
                        .addAttribute("email", jwt.getClaim("email"))
                        .addAttribute("type", jwt.getClaim("type"))
                        .addAttribute("client_id", jwt.getClaim("client_id"))
                        .build();

            } catch (ParseException e) {
                LOG.debug("Failed to parse JWT token", e);
                return null;
            } catch (Exception e) {
                LOG.warn("Error during token authentication: " + e.getMessage(), e);
                return null;
            }
        });
    }
}
