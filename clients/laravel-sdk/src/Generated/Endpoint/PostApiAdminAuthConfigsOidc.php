<?php

namespace FlowCatalyst\Generated\Endpoint;

class PostApiAdminAuthConfigsOidc extends \FlowCatalyst\Generated\Runtime\Client\BaseEndpoint implements \FlowCatalyst\Generated\Runtime\Client\Endpoint
{
    /**
     * @param null|\FlowCatalyst\Generated\Model\ApiAdminAuthConfigsOidcPostBody $requestBody
     */
    public function __construct(?\FlowCatalyst\Generated\Model\ApiAdminAuthConfigsOidcPostBody $requestBody = null)
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
        return '/api/admin/auth-configs/oidc';
    }
    public function getBody(\Symfony\Component\Serializer\SerializerInterface $serializer, $streamFactory = null): array
    {
        if ($this->body instanceof \FlowCatalyst\Generated\Model\ApiAdminAuthConfigsOidcPostBody) {
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
     * @throws \FlowCatalyst\Generated\Exception\PostApiAdminAuthConfigsOidcBadRequestException
     * @throws \FlowCatalyst\Generated\Exception\PostApiAdminAuthConfigsOidcConflictException
     *
     * @return null|\FlowCatalyst\Generated\Model\ApiAdminAuthConfigsOidcPostResponse201
     */
    protected function transformResponseBody(\Psr\Http\Message\ResponseInterface $response, \Symfony\Component\Serializer\SerializerInterface $serializer, ?string $contentType = null)
    {
        $status = $response->getStatusCode();
        $body = (string) $response->getBody();
        if (is_null($contentType) === false && (201 === $status && mb_strpos(strtolower($contentType), 'application/json') !== false)) {
            return $serializer->deserialize($body, 'FlowCatalyst\Generated\Model\ApiAdminAuthConfigsOidcPostResponse201', 'json');
        }
        if (is_null($contentType) === false && (400 === $status && mb_strpos(strtolower($contentType), 'application/json') !== false)) {
            throw new \FlowCatalyst\Generated\Exception\PostApiAdminAuthConfigsOidcBadRequestException($serializer->deserialize($body, 'FlowCatalyst\Generated\Model\ApiAdminAuthConfigsOidcPostResponse400', 'json'), $response);
        }
        if (is_null($contentType) === false && (409 === $status && mb_strpos(strtolower($contentType), 'application/json') !== false)) {
            throw new \FlowCatalyst\Generated\Exception\PostApiAdminAuthConfigsOidcConflictException($serializer->deserialize($body, 'FlowCatalyst\Generated\Model\ApiAdminAuthConfigsOidcPostResponse409', 'json'), $response);
        }
    }
    public function getAuthenticationScopes(): array
    {
        return ['bearerAuth'];
    }
}