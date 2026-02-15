<?php

namespace FlowCatalyst\Generated\Endpoint;

class GetEntityAuditLogs extends \FlowCatalyst\Generated\Runtime\Client\BaseEndpoint implements \FlowCatalyst\Generated\Runtime\Client\Endpoint
{
    protected $entityId;
    protected $entityType;
    /**
     * Returns all audit logs for a specific entity
     * @param string $entityId
     * @param string $entityType
     */
    public function __construct(string $entityId, string $entityType)
    {
        $this->entityId = $entityId;
        $this->entityType = $entityType;
    }
    use \FlowCatalyst\Generated\Runtime\Client\EndpointTrait;
    public function getMethod(): string
    {
        return 'GET';
    }
    public function getUri(): string
    {
        return str_replace(['{entityId}', '{entityType}'], [$this->entityId, $this->entityType], '/api/admin/audit-logs/entity/{entityType}/{entityId}');
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
     * @throws \FlowCatalyst\Generated\Exception\GetEntityAuditLogsUnauthorizedException
     * @throws \FlowCatalyst\Generated\Exception\GetEntityAuditLogsForbiddenException
     *
     * @return null|\FlowCatalyst\Generated\Model\EntityAuditLogsResponse
     */
    protected function transformResponseBody(\Psr\Http\Message\ResponseInterface $response, \Symfony\Component\Serializer\SerializerInterface $serializer, ?string $contentType = null)
    {
        $status = $response->getStatusCode();
        $body = (string) $response->getBody();
        if (is_null($contentType) === false && (200 === $status && mb_strpos(strtolower($contentType), 'application/json') !== false)) {
            return $serializer->deserialize($body, 'FlowCatalyst\Generated\Model\EntityAuditLogsResponse', 'json');
        }
        if (is_null($contentType) === false && (401 === $status && mb_strpos(strtolower($contentType), 'application/json') !== false)) {
            throw new \FlowCatalyst\Generated\Exception\GetEntityAuditLogsUnauthorizedException($serializer->deserialize($body, 'FlowCatalyst\Generated\Model\UnauthorizedResponse', 'json'), $response);
        }
        if (is_null($contentType) === false && (403 === $status && mb_strpos(strtolower($contentType), 'application/json') !== false)) {
            throw new \FlowCatalyst\Generated\Exception\GetEntityAuditLogsForbiddenException($serializer->deserialize($body, 'FlowCatalyst\Generated\Model\ForbiddenResponse', 'json'), $response);
        }
    }
    public function getAuthenticationScopes(): array
    {
        return [];
    }
}