<?php

namespace FlowCatalyst\Generated\Endpoint;

class PostApiDispatchJobsBatch extends \FlowCatalyst\Generated\Runtime\Client\BaseEndpoint implements \FlowCatalyst\Generated\Runtime\Client\Endpoint
{
    /**
     * Creates multiple dispatch jobs in a single operation. Maximum batch size is 100 jobs.
     * @param null|\FlowCatalyst\Generated\Model\CreateDispatchJobRequest[] $requestBody
     */
    public function __construct(?array $requestBody = null)
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
        return '/api/dispatch/jobs/batch';
    }
    public function getBody(\Symfony\Component\Serializer\SerializerInterface $serializer, $streamFactory = null): array
    {
        if (is_array($this->body) and isset($this->body[0]) and $this->body[0] instanceof \FlowCatalyst\Generated\Model\CreateDispatchJobRequest) {
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
     * @throws \FlowCatalyst\Generated\Exception\PostApiDispatchJobsBatchBadRequestException
     * @throws \FlowCatalyst\Generated\Exception\PostApiDispatchJobsBatchInternalServerErrorException
     *
     * @return null|\FlowCatalyst\Generated\Model\BatchDispatchJobResponse
     */
    protected function transformResponseBody(\Psr\Http\Message\ResponseInterface $response, \Symfony\Component\Serializer\SerializerInterface $serializer, ?string $contentType = null)
    {
        $status = $response->getStatusCode();
        $body = (string) $response->getBody();
        if (is_null($contentType) === false && (201 === $status && mb_strpos(strtolower($contentType), 'application/json') !== false)) {
            return $serializer->deserialize($body, 'FlowCatalyst\Generated\Model\BatchDispatchJobResponse', 'json');
        }
        if (is_null($contentType) === false && (400 === $status && mb_strpos(strtolower($contentType), 'application/json') !== false)) {
            throw new \FlowCatalyst\Generated\Exception\PostApiDispatchJobsBatchBadRequestException($serializer->deserialize($body, 'FlowCatalyst\Generated\Model\ErrorResponse3', 'json'), $response);
        }
        if (is_null($contentType) === false && (500 === $status && mb_strpos(strtolower($contentType), 'application/json') !== false)) {
            throw new \FlowCatalyst\Generated\Exception\PostApiDispatchJobsBatchInternalServerErrorException($serializer->deserialize($body, 'FlowCatalyst\Generated\Model\ErrorResponse3', 'json'), $response);
        }
    }
    public function getAuthenticationScopes(): array
    {
        return [];
    }
}