<?php

namespace FlowCatalyst\Generated\Endpoint;

class PostApiDispatchProcess extends \FlowCatalyst\Generated\Runtime\Client\BaseEndpoint implements \FlowCatalyst\Generated\Runtime\Client\Endpoint
{
    /**
     * Internal endpoint that executes webhook dispatch and records attempts. Requires HMAC-SHA256 authentication via Bearer token.
     * @param null|\FlowCatalyst\Generated\Model\ProcessRequest $requestBody
     */
    public function __construct(?\FlowCatalyst\Generated\Model\ProcessRequest $requestBody = null)
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
        return '/api/dispatch/process';
    }
    public function getBody(\Symfony\Component\Serializer\SerializerInterface $serializer, $streamFactory = null): array
    {
        if ($this->body instanceof \FlowCatalyst\Generated\Model\ProcessRequest) {
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
     * @throws \FlowCatalyst\Generated\Exception\PostApiDispatchProcessUnauthorizedException
     * @throws \FlowCatalyst\Generated\Exception\PostApiDispatchProcessInternalServerErrorException
     * @throws \FlowCatalyst\Generated\Exception\PostApiDispatchProcessBadRequestException
     *
     * @return null|\FlowCatalyst\Generated\Model\ProcessResponse
     */
    protected function transformResponseBody(\Psr\Http\Message\ResponseInterface $response, \Symfony\Component\Serializer\SerializerInterface $serializer, ?string $contentType = null)
    {
        $status = $response->getStatusCode();
        $body = (string) $response->getBody();
        if (is_null($contentType) === false && (200 === $status && mb_strpos(strtolower($contentType), 'application/json') !== false)) {
            return $serializer->deserialize($body, 'FlowCatalyst\Generated\Model\ProcessResponse', 'json');
        }
        if (is_null($contentType) === false && (401 === $status && mb_strpos(strtolower($contentType), 'application/json') !== false)) {
            throw new \FlowCatalyst\Generated\Exception\PostApiDispatchProcessUnauthorizedException($serializer->deserialize($body, 'FlowCatalyst\Generated\Model\ProcessResponse', 'json'), $response);
        }
        if (is_null($contentType) === false && (500 === $status && mb_strpos(strtolower($contentType), 'application/json') !== false)) {
            throw new \FlowCatalyst\Generated\Exception\PostApiDispatchProcessInternalServerErrorException($serializer->deserialize($body, 'FlowCatalyst\Generated\Model\ProcessResponse', 'json'), $response);
        }
        if (400 === $status) {
            throw new \FlowCatalyst\Generated\Exception\PostApiDispatchProcessBadRequestException($response);
        }
    }
    public function getAuthenticationScopes(): array
    {
        return [];
    }
}