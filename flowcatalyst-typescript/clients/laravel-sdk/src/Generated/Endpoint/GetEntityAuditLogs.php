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
        if (401 === $status) {
            throw new \FlowCatalyst\Generated\Exception\GetEntityAuditLogsUnauthorizedException($response);
        }
    }
    public function getAuthenticationScopes(): array
    {
        return [];
    }
}