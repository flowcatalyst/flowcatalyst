package tech.flowcatalyst.platform.principal.operations.createuser;

import jakarta.enterprise.context.ApplicationScoped;
import jakarta.inject.Inject;
import tech.flowcatalyst.platform.authentication.AuthProvider;
import tech.flowcatalyst.platform.authentication.IdpType;
import tech.flowcatalyst.platform.client.ClientAuthConfig;
import tech.flowcatalyst.platform.client.ClientAuthConfigRepository;
import tech.flowcatalyst.platform.common.ExecutionContext;
import tech.flowcatalyst.platform.common.Result;
import tech.flowcatalyst.platform.common.UnitOfWork;
import tech.flowcatalyst.platform.common.errors.UseCaseError;
import tech.flowcatalyst.platform.principal.AnchorDomainRepository;
import tech.flowcatalyst.platform.principal.PasswordService;
import tech.flowcatalyst.platform.principal.Principal;
import tech.flowcatalyst.platform.principal.PrincipalRepository;
import tech.flowcatalyst.platform.principal.PrincipalType;
import tech.flowcatalyst.platform.principal.UserIdentity;
import tech.flowcatalyst.platform.principal.events.UserCreated;
import tech.flowcatalyst.platform.shared.EntityType;
import tech.flowcatalyst.platform.shared.TsidGenerator;

import java.util.Map;
import java.util.Optional;

/**
 * Use case for creating a new user.
 */
@ApplicationScoped
public class CreateUserUseCase {

    @Inject
    PrincipalRepository principalRepo;

    @Inject
    PasswordService passwordService;

    @Inject
    AnchorDomainRepository anchorDomainRepo;

    @Inject
    ClientAuthConfigRepository authConfigRepo;

    @Inject
    UnitOfWork unitOfWork;

    public Result<UserCreated> execute(CreateUserCommand command, ExecutionContext context) {
        // Validate email
        if (command.email() == null || command.email().isBlank()) {
            return Result.failure(new UseCaseError.ValidationError(
                "EMAIL_REQUIRED",
                "Email is required",
                Map.of()
            ));
        }

        // Validate email format
        if (!isValidEmail(command.email())) {
            return Result.failure(new UseCaseError.ValidationError(
                "INVALID_EMAIL",
                "Invalid email format",
                Map.of("email", command.email())
            ));
        }

        // Check if email already exists
        if (principalRepo.findByEmail(command.email()).isPresent()) {
            return Result.failure(new UseCaseError.BusinessRuleViolation(
                "EMAIL_EXISTS",
                "Email already exists",
                Map.of("email", command.email())
            ));
        }

        // Validate name
        if (command.name() == null || command.name().isBlank()) {
            return Result.failure(new UseCaseError.ValidationError(
                "NAME_REQUIRED",
                "Name is required",
                Map.of()
            ));
        }

        // Extract domain from email
        String emailDomain = extractDomain(command.email());

        // Check if anchor domain user
        boolean isAnchorUser = anchorDomainRepo.existsByDomain(emailDomain);

        // Determine IdP type based on domain auth configuration
        Optional<ClientAuthConfig> authConfig = authConfigRepo.findByEmailDomain(emailDomain);
        IdpType idpType;
        String passwordHash = null;

        if (authConfig.isPresent() && authConfig.get().authProvider == AuthProvider.OIDC) {
            // OIDC user - no password required
            idpType = IdpType.OIDC;
            // Password should not be provided for OIDC users
            if (command.password() != null && !command.password().isBlank()) {
                return Result.failure(new UseCaseError.ValidationError(
                    "PASSWORD_NOT_ALLOWED",
                    "Password should not be provided for OIDC users",
                    Map.of("authProvider", "OIDC")
                ));
            }
        } else {
            // Internal auth user - password required
            idpType = IdpType.INTERNAL;
            try {
                passwordHash = passwordService.validateAndHashPassword(command.password());
            } catch (IllegalArgumentException e) {
                return Result.failure(new UseCaseError.ValidationError(
                    "INVALID_PASSWORD",
                    e.getMessage(),
                    Map.of()
                ));
            }
        }

        // Create principal
        Principal principal = new Principal();
        principal.id = TsidGenerator.generate(EntityType.PRINCIPAL);
        principal.type = PrincipalType.USER;
        principal.clientId = command.clientId();
        principal.name = command.name();
        principal.active = true;

        // Create user identity
        UserIdentity userIdentity = new UserIdentity();
        userIdentity.email = command.email();
        userIdentity.emailDomain = emailDomain;
        userIdentity.idpType = idpType;
        userIdentity.passwordHash = passwordHash;

        principal.userIdentity = userIdentity;

        // Create domain event
        UserCreated event = UserCreated.fromContext(context)
            .userId(principal.id)
            .email(principal.userIdentity.email)
            .emailDomain(emailDomain)
            .name(principal.name)
            .clientId(principal.clientId)
            .idpType(idpType)
            .isAnchorUser(isAnchorUser)
            .build();

        // Commit atomically
        return unitOfWork.commit(principal, event, command);
    }

    private boolean isValidEmail(String email) {
        return email != null && email.contains("@") && email.indexOf("@") > 0
            && email.indexOf("@") < email.length() - 1;
    }

    private String extractDomain(String email) {
        int atIndex = email.indexOf('@');
        return email.substring(atIndex + 1).toLowerCase();
    }
}
