<?php

namespace FlowCatalyst\Generated\Endpoint;

class PostAuthRefresh extends \FlowCatalyst\Generated\Runtime\Client\BaseEndpoint implements \FlowCatalyst\Generated\Runtime\Client\Endpoint
{
    /**
     * Exchange a refresh token for a new access token.
     * The refresh token is rotated (old one invalidated, new one issued).
     * @param null|\FlowCatalyst\Generated\Model\RefreshTokenRequest $requestBody
     */
    public function __construct(?\FlowCatalyst\Generated\Model\RefreshTokenRequest $requestBody = null)
    {
        $this->body = $requestBody;
    }
    use \FlowCatalyst\Generated\Runtime\Client\EndpointTrait;
    public function getMethod(): string
    {
        return 'POST';
    }
    public function getUri(): string
    {
        return '/auth/refresh';
    }
    public function getBody(\Symfony\Component\Serializer\SerializerInterface $serializer, $streamFactory = null): array
    {
        if ($this->body instanceof \FlowCatalyst\Generated\Model\RefreshTokenRequest) {
            return [['Content-Type' => ['application/json']], $serializer->serialize($this->body, 'json')];
        }
        return [[], null];
    }
    public function getExtraHeaders(): array
    {
        return ['Accept' => ['application/json']];
    }
    /**
     * {@inheritdoc}
     *
     * @throws \FlowCatalyst\Generated\Exception\PostAuthRefreshUnauthorizedException
     *
     * @return null|\FlowCatalyst\Generated\Model\TokenRefreshResponse
     */
    protected function transformResponseBody(\Psr\Http\Message\ResponseInterface $response, \Symfony\Component\Serializer\SerializerInterface $serializer, ?string $contentType = null)
    {
        $status = $response->getStatusCode();
        $body = (string) $response->getBody();
        if (is_null($contentType) === false && (200 === $status && mb_strpos(strtolower($contentType), 'application/json') !== false)) {
            return $serializer->deserialize($body, 'FlowCatalyst\Generated\Model\TokenRefreshResponse', 'json');
        }
        if (401 === $status) {
            throw new \FlowCatalyst\Generated\Exception\PostAuthRefreshUnauthorizedException($response);
        }
    }
    public function getAuthenticationScopes(): array
    {
        return [];
    }
}