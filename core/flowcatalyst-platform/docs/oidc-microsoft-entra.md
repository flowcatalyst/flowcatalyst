# Microsoft Entra ID (Azure AD) OIDC Configuration

This guide explains how to configure FlowCatalyst to authenticate users via Microsoft Entra ID using the Domain IDP configuration.

## Overview

FlowCatalyst uses **Domain IDPs** to map email domains to identity providers. When a user logs in with an email like `user@contoso.com`, FlowCatalyst looks up the auth configuration for `contoso.com` to determine how to authenticate them.

For Microsoft Entra multi-tenant, you configure:
- **Issuer URL**: The Microsoft Entra authorization endpoint
- **Client ID**: Your Azure app registration client ID
- **Client Secret**: Stored as a secret reference
- **Multi-Tenant**: Enabled, with an issuer pattern for validation

## Microsoft Entra App Registration

### Step 1: Create App Registration

1. Go to [Azure Portal](https://portal.azure.com)
2. Navigate to **Microsoft Entra ID** → **App registrations**
3. Click **New registration**
4. Configure:
   - **Name**: `FlowCatalyst` (or your preferred name)
   - **Supported account types**:
     - **Multitenant**: "Accounts in any organizational directory"
   - **Redirect URI**:
     - Type: `Web`
     - URI: `https://your-flowcatalyst-domain.com/oauth/callback`

5. Click **Register**

### Step 2: Note the Application Details

From the **Overview** page, copy these values:

| Field | Where to Find |
|-------|---------------|
| **Application (client) ID** | Overview → "Application (client) ID" |

### Step 3: Create Client Secret

1. Go to **Certificates & secrets**
2. Click **New client secret**
3. Add a description and expiry period
4. Click **Add**
5. **Copy the secret value immediately** (it won't be shown again)

Store this secret in your secrets manager (AWS Secrets Manager, Vault, etc.)

### Step 4: Configure API Permissions

1. Go to **API permissions**
2. Ensure these permissions are granted:
   - `openid` - Sign in users
   - `email` - View users' email address
   - `profile` - View users' basic profile

3. Click **Grant admin consent** if required for your organization

## FlowCatalyst Configuration

### Creating the Domain IDP

1. Go to **Authentication** → **Domain IDPs** in the FlowCatalyst UI
2. Click **Add Domain IDP**
3. Enter the email domain (e.g., `contoso.com`)
4. Select **OIDC** as the auth provider
5. Configure the OIDC settings:

### OIDC Configuration Fields

| Field | Value for Microsoft Entra Multi-Tenant |
|-------|----------------------------------------|
| **Issuer URL** | `https://login.microsoftonline.com/common/v2.0` |
| **Client ID** | Your Application (client) ID from Azure |
| **Client Secret Reference** | Secret reference (see below) |
| **Multi-Tenant** | ✅ Enabled |
| **Issuer Pattern** | `https://login.microsoftonline.com/{tenantId}/v2.0` |

### Secret Reference Formats

The client secret must be stored in a secrets manager and referenced using one of these formats:

| Provider | Format | Example |
|----------|--------|---------|
| AWS Secrets Manager | `aws-sm://secret-name` | `aws-sm://flowcatalyst/entra-client-secret` |
| AWS Parameter Store | `aws-ps://parameter-name` | `aws-ps://flowcatalyst/entra-secret` |
| HashiCorp Vault | `vault://path/to/secret#key` | `vault://secret/flowcatalyst#entra-client-secret` |
| GCP Secret Manager | `gcp-sm://project/secret-name` | `gcp-sm://my-project/entra-secret` |

### Multi-Tenant Issuer Pattern

When **Multi-Tenant** is enabled, the **Issuer Pattern** field appears. This is used to validate tokens from any Azure AD tenant.

For Microsoft Entra, use:
```
https://login.microsoftonline.com/{tenantId}/v2.0
```

The `{tenantId}` placeholder is replaced with the actual tenant ID from the token during validation.

## How It Works

1. User enters email `user@contoso.com` on the login page
2. FlowCatalyst looks up the Domain IDP for `contoso.com`
3. User is redirected to Microsoft Entra (`login.microsoftonline.com/common/...`)
4. User authenticates with their Microsoft account
5. Microsoft returns an ID token to FlowCatalyst
6. FlowCatalyst validates the token:
   - Signature verified against Microsoft's JWKS
   - Issuer matches the pattern (e.g., `https://login.microsoftonline.com/abc123-tenant-id/v2.0`)
   - Audience matches the Client ID
7. User is created/updated in FlowCatalyst with the appropriate scope

## Access Configuration

After creating the Domain IDP, configure the **Access Configuration**:

| Config Type | Description | Use Case |
|-------------|-------------|----------|
| **Anchor** | Platform-wide access to all clients | Your organization's admins |
| **Client** | Bound to a specific client | Customer organizations |
| **Partner** | Access to granted clients only | Partners with multi-client access |

### Example: Anchor Domain for Your Org

If `contoso.com` is your organization and users should be platform admins:

1. Create Domain IDP for `contoso.com` with OIDC settings
2. Set **Config Type** to **Anchor**
3. Users from `@contoso.com` will have ANCHOR scope

### Example: Client Domain for Customers

If `acme.com` users should access only the Acme client:

1. Create Domain IDP for `acme.com` with OIDC settings
2. Set **Config Type** to **Client**
3. Select **Acme Corporation** as the Primary Client
4. Users from `@acme.com` will have CLIENT scope for Acme

## Microsoft Entra Endpoints Reference

### Multi-Tenant Endpoints (use `common`)

```
Authorization: https://login.microsoftonline.com/common/oauth2/v2.0/authorize
Token:         https://login.microsoftonline.com/common/oauth2/v2.0/token
JWKS:          https://login.microsoftonline.com/common/discovery/v2.0/keys
Discovery:     https://login.microsoftonline.com/common/v2.0/.well-known/openid-configuration
```

### Single Tenant Endpoints

Replace `{tenant-id}` with your Directory (tenant) ID:

```
Authorization: https://login.microsoftonline.com/{tenant-id}/oauth2/v2.0/authorize
Token:         https://login.microsoftonline.com/{tenant-id}/oauth2/v2.0/token
JWKS:          https://login.microsoftonline.com/{tenant-id}/discovery/v2.0/keys
Discovery:     https://login.microsoftonline.com/{tenant-id}/v2.0/.well-known/openid-configuration
```

## Troubleshooting

### Token Validation Fails

**"Invalid issuer" error**
- Ensure **Multi-Tenant** is enabled
- Check that **Issuer Pattern** is set to `https://login.microsoftonline.com/{tenantId}/v2.0`

**"Invalid audience" error**
- Verify the **Client ID** matches your Azure app registration
- Ensure you're using v2.0 endpoints

### User Not Created

- Check that the user's email domain matches the Domain IDP configuration
- Verify the `email` claim is present in the token (check Azure API permissions)

### CORS Errors

- In FlowCatalyst, ensure your OAuth Client has the correct **Allowed Origins**
- In Azure, verify the **Redirect URI** is correct

### Useful Links

- **Azure App Registrations**: [portal.azure.com](https://portal.azure.com/#blade/Microsoft_AAD_IAM/ActiveDirectoryMenuBlade/RegisteredApps)
- **OpenID Configuration**: `https://login.microsoftonline.com/common/v2.0/.well-known/openid-configuration`
- **Token Decoder**: [jwt.ms](https://jwt.ms)

## Security Best Practices

1. **Store secrets securely** - Never hardcode client secrets; use secret references
2. **Use HTTPS** - All redirect URIs must use HTTPS in production
3. **Rotate secrets** - Set calendar reminders to rotate secrets before expiry
4. **Minimum permissions** - Only request `openid`, `email`, `profile` scopes
5. **Validate secret references** - Use the "Validate Secret" button in the UI to test
