<?php

namespace FlowCatalyst\Generated\Endpoint;

class GetOauthAuthorize extends \FlowCatalyst\Generated\Runtime\Client\BaseEndpoint implements \FlowCatalyst\Generated\Runtime\Client\Endpoint
{
    /**
     * @param array{
     *    "client_id"?: string, //OAuth client ID
     *    "code_challenge"?: string, //PKCE code challenge
     *    "code_challenge_method"?: string, //PKCE challenge method (S256 or plain)
     *    "nonce"?: string, //OIDC nonce for replay protection
     *    "redirect_uri"?: string, //URI to redirect after authorization
     *    "response_type"?: string, //Must be 'code'
     *    "scope"?: string, //Requested scopes (space-separated)
     *    "state"?: string, //Client state for CSRF protection
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
        return '/oauth/authorize';
    }
    public function getBody(\Symfony\Component\Serializer\SerializerInterface $serializer, $streamFactory = null): array
    {
        return [[], null];
    }
    public function getExtraHeaders(): array
    {
        if (empty($this->accept)) {
            return ['Accept' => []];
        }
        return $this->accept;
    }
    protected function getQueryOptionsResolver(): \Symfony\Component\OptionsResolver\OptionsResolver
    {
        $optionsResolver = parent::getQueryOptionsResolver();
        $optionsResolver->setDefined(['client_id', 'code_challenge', 'code_challenge_method', 'nonce', 'redirect_uri', 'response_type', 'scope', 'state']);
        $optionsResolver->setRequired([]);
        $optionsResolver->setDefaults(['code_challenge_method' => 'S256']);
        $optionsResolver->addAllowedTypes('client_id', ['string']);
        $optionsResolver->addAllowedTypes('code_challenge', ['string']);
        $optionsResolver->addAllowedTypes('code_challenge_method', ['string']);
        $optionsResolver->addAllowedTypes('nonce', ['string']);
        $optionsResolver->addAllowedTypes('redirect_uri', ['string']);
        $optionsResolver->addAllowedTypes('response_type', ['string']);
        $optionsResolver->addAllowedTypes('scope', ['string']);
        $optionsResolver->addAllowedTypes('state', ['string']);
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
        if (200 === $status) {
        }
    }
    public function getAuthenticationScopes(): array
    {
        return [];
    }
}