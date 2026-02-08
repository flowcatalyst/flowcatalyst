<?php

namespace FlowCatalyst\Generated\Endpoint;

class GetAuthOidcLogin extends \FlowCatalyst\Generated\Runtime\Client\BaseEndpoint implements \FlowCatalyst\Generated\Runtime\Client\Endpoint
{
    /**
     * Redirects to external IDP for authentication
     * @param array{
     *    "domain": string, //Email domain to authenticate
     *    "oauth_client_id"?: string,
     *    "oauth_code_challenge"?: string,
     *    "oauth_code_challenge_method"?: string,
     *    "oauth_nonce"?: string,
     *    "oauth_redirect_uri"?: string,
     *    "oauth_scope"?: string,
     *    "oauth_state"?: string,
     *    "return_url"?: string, //URL to return to after login
     * } $queryParameters
     */
    public function __construct(array $queryParameters = [])
    {
        $this->queryParameters = $queryParameters;
    }
    use \FlowCatalyst\Generated\Runtime\Client\EndpointTrait;
    public function getMethod(): string
    {
        return 'GET';
    }
    public function getUri(): string
    {
        return '/auth/oidc/login';
    }
    public function getBody(\Symfony\Component\Serializer\SerializerInterface $serializer, $streamFactory = null): array
    {
        return [[], null];
    }
    public function getExtraHeaders(): array
    {
        return ['Accept' => ['application/json']];
    }
    protected function getQueryOptionsResolver(): \Symfony\Component\OptionsResolver\OptionsResolver
    {
        $optionsResolver = parent::getQueryOptionsResolver();
        $optionsResolver->setDefined(['domain', 'oauth_client_id', 'oauth_code_challenge', 'oauth_code_challenge_method', 'oauth_nonce', 'oauth_redirect_uri', 'oauth_scope', 'oauth_state', 'return_url']);
        $optionsResolver->setRequired(['domain']);
        $optionsResolver->setDefaults([]);
        $optionsResolver->addAllowedTypes('domain', ['string']);
        $optionsResolver->addAllowedTypes('oauth_client_id', ['string']);
        $optionsResolver->addAllowedTypes('oauth_code_challenge', ['string']);
        $optionsResolver->addAllowedTypes('oauth_code_challenge_method', ['string']);
        $optionsResolver->addAllowedTypes('oauth_nonce', ['string']);
        $optionsResolver->addAllowedTypes('oauth_redirect_uri', ['string']);
        $optionsResolver->addAllowedTypes('oauth_scope', ['string']);
        $optionsResolver->addAllowedTypes('oauth_state', ['string']);
        $optionsResolver->addAllowedTypes('return_url', ['string']);
        return $optionsResolver;
    }
    /**
     * {@inheritdoc}
     *
     *
     * @return null
     */
    protected function transformResponseBody(\Psr\Http\Message\ResponseInterface $response, \Symfony\Component\Serializer\SerializerInterface $serializer, ?string $contentType = null)
    {
        $status = $response->getStatusCode();
        $body = (string) $response->getBody();
        if (is_null($contentType) === false && (200 === $status && mb_strpos(strtolower($contentType), 'application/json') !== false)) {
            return json_decode($body);
        }
    }
    public function getAuthenticationScopes(): array
    {
        return [];
    }
}