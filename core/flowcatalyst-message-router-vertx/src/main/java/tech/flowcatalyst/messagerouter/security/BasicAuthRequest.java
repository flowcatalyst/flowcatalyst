package tech.flowcatalyst.messagerouter.security;

/**
 * BasicAuth authentication request carrying username and password.
 */
public class BasicAuthRequest {
    private final String username;
    private final String password;

    public BasicAuthRequest(String username, String password) {
        this.username = username;
        this.password = password;
    }

    public String getUsername() {
        return username;
    }

    public String getPassword() {
        return password;
    }
}
