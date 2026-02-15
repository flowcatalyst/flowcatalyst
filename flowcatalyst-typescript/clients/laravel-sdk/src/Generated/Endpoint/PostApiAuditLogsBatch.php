<?php

namespace FlowCatalyst\Generated\Endpoint;

class PostApiAuditLogsBatch extends \FlowCatalyst\Generated\Runtime\Client\BaseEndpoint implements \FlowCatalyst\Generated\Runtime\Client\Endpoint
{
    /**
     * Creates multiple audit log entries in a single operation. Maximum batch size is 100.
     * @param null|\FlowCatalyst\Generated\Model\CreateAuditLogRequest[] $requestBody
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
        return '/api/audit-logs/batch';
    }
    public function getBody(\Symfony\Component\Serializer\SerializerInterface $serializer, $streamFactory = null): array
    {
        if (is_array($this->body) and isset($this->body[0]) and $this->body[0] instanceof \FlowCatalyst\Generated\Model\CreateAuditLogRequest) {
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
     * @throws \FlowCatalyst\Generated\Exception\PostApiAuditLogsBatchBadRequestException
     *
     * @return null|\FlowCatalyst\Generated\Model\BatchAuditLogResponse
     */
    protected function transformResponseBody(\Psr\Http\Message\ResponseInterface $response, \Symfony\Component\Serializer\SerializerInterface $serializer, ?string $contentType = null)
    {
        $status = $response->getStatusCode();
        $body = (string) $response->getBody();
        if (is_null($contentType) === false && (201 === $status && mb_strpos(strtolower($contentType), 'application/json') !== false)) {
            return $serializer->deserialize($body, 'FlowCatalyst\Generated\Model\BatchAuditLogResponse', 'json');
        }
        if (is_null($contentType) === false && (400 === $status && mb_strpos(strtolower($contentType), 'application/json') !== false)) {
            throw new \FlowCatalyst\Generated\Exception\PostApiAuditLogsBatchBadRequestException($serializer->deserialize($body, 'FlowCatalyst\Generated\Model\ErrorResponse5', 'json'), $response);
        }
    }
    public function getAuthenticationScopes(): array
    {
        return [];
    }
}