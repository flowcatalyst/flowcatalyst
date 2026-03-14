<?php

namespace FlowCatalyst\Generated\Endpoint;

class PostApiAdminAuthConfigsInternal extends \FlowCatalyst\Generated\Runtime\Client\BaseEndpoint implements \FlowCatalyst\Generated\Runtime\Client\Endpoint
{
    /**
     * @param null|\FlowCatalyst\Generated\Model\ApiAdminAuthConfigsInternalPostBody $requestBody
     */
    public function __construct(?\FlowCatalyst\Generated\Model\ApiAdminAuthConfigsInternalPostBody $requestBody = null)
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
        return '/api/admin/auth-configs/internal';
    }
    public function getBody(\Symfony\Component\Serializer\SerializerInterface $serializer, $streamFactory = null): array
    {
        if ($this->body instanceof \FlowCatalyst\Generated\Model\ApiAdminAuthConfigsInternalPostBody) {
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
     * @throws \FlowCatalyst\Generated\Exception\PostApiAdminAuthConfigsInternalBadRequestException
     * @throws \FlowCatalyst\Generated\Exception\PostApiAdminAuthConfigsInternalConflictException
     *
     * @return null|\FlowCatalyst\Generated\Model\ApiAdminAuthConfigsInternalPostResponse201
     */
    protected function transformResponseBody(\Psr\Http\Message\ResponseInterface $response, \Symfony\Component\Serializer\SerializerInterface $serializer, ?string $contentType = null)
    {
        $status = $response->getStatusCode();
        $body = (string) $response->getBody();
        if (is_null($contentType) === false && (201 === $status && mb_strpos(strtolower($contentType), 'application/json') !== false)) {
            return $serializer->deserialize($body, 'FlowCatalyst\Generated\Model\ApiAdminAuthConfigsInternalPostResponse201', 'json');
        }
        if (is_null($contentType) === false && (400 === $status && mb_strpos(strtolower($contentType), 'application/json') !== false)) {
            throw new \FlowCatalyst\Generated\Exception\PostApiAdminAuthConfigsInternalBadRequestException($serializer->deserialize($body, 'FlowCatalyst\Generated\Model\ApiAdminAuthConfigsInternalPostResponse400', 'json'), $response);
        }
        if (is_null($contentType) === false && (409 === $status && mb_strpos(strtolower($contentType), 'application/json') !== false)) {
            throw new \FlowCatalyst\Generated\Exception\PostApiAdminAuthConfigsInternalConflictException($serializer->deserialize($body, 'FlowCatalyst\Generated\Model\ApiAdminAuthConfigsInternalPostResponse409', 'json'), $response);
        }
    }
    public function getAuthenticationScopes(): array
    {
        return ['bearerAuth'];
    }
}