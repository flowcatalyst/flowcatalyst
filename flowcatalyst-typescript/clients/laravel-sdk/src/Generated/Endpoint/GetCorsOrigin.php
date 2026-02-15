<?php

namespace FlowCatalyst\Generated\Endpoint;

class GetCorsOrigin extends \FlowCatalyst\Generated\Runtime\Client\BaseEndpoint implements \FlowCatalyst\Generated\Runtime\Client\Endpoint
{
    protected $id;
    /**
     * @param string $id
     */
    public function __construct(string $id)
    {
        $this->id = $id;
    }
    use \FlowCatalyst\Generated\Runtime\Client\EndpointTrait;
    public function getMethod(): string
    {
        return 'GET';
    }
    public function getUri(): string
    {
        return str_replace(['{id}'], [$this->id], '/api/admin/platform/cors/{id}');
    }
    public function getBody(\Symfony\Component\Serializer\SerializerInterface $serializer, $streamFactory = null): array
    {
        return [[], null];
    }
    public function getExtraHeaders(): array
    {
        return ['Accept' => ['application/json']];
    }
    /**
     * {@inheritdoc}
     *
     * @throws \FlowCatalyst\Generated\Exception\GetCorsOriginNotFoundException
     * @throws \FlowCatalyst\Generated\Exception\GetCorsOriginUnauthorizedException
     * @throws \FlowCatalyst\Generated\Exception\GetCorsOriginForbiddenException
     *
     * @return null|\FlowCatalyst\Generated\Model\CorsOriginDto
     */
    protected function transformResponseBody(\Psr\Http\Message\ResponseInterface $response, \Symfony\Component\Serializer\SerializerInterface $serializer, ?string $contentType = null)
    {
        $status = $response->getStatusCode();
        $body = (string) $response->getBody();
        if (is_null($contentType) === false && (200 === $status && mb_strpos(strtolower($contentType), 'application/json') !== false)) {
            return $serializer->deserialize($body, 'FlowCatalyst\Generated\Model\CorsOriginDto', 'json');
        }
        if (is_null($contentType) === false && (404 === $status && mb_strpos(strtolower($contentType), 'application/json') !== false)) {
            throw new \FlowCatalyst\Generated\Exception\GetCorsOriginNotFoundException($serializer->deserialize($body, 'FlowCatalyst\Generated\Model\NotFoundResponse', 'json'), $response);
        }
        if (is_null($contentType) === false && (401 === $status && mb_strpos(strtolower($contentType), 'application/json') !== false)) {
            throw new \FlowCatalyst\Generated\Exception\GetCorsOriginUnauthorizedException($serializer->deserialize($body, 'FlowCatalyst\Generated\Model\UnauthorizedResponse', 'json'), $response);
        }
        if (is_null($contentType) === false && (403 === $status && mb_strpos(strtolower($contentType), 'application/json') !== false)) {
            throw new \FlowCatalyst\Generated\Exception\GetCorsOriginForbiddenException($serializer->deserialize($body, 'FlowCatalyst\Generated\Model\ForbiddenResponse', 'json'), $response);
        }
    }
    public function getAuthenticationScopes(): array
    {
        return [];
    }
}