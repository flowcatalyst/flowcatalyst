<?php

namespace FlowCatalyst\Generated\Endpoint;

class ListAuditLogs extends \FlowCatalyst\Generated\Runtime\Client\BaseEndpoint implements \FlowCatalyst\Generated\Runtime\Client\Endpoint
{
    /**
     * Returns audit logs with optional filtering by entity type, entity ID, principal, or operation
     * @param array{
     *    "entityId"?: string, //Filter by entity ID
     *    "entityType"?: string, //Filter by entity type (e.g., 'IdentityProvider', 'Role')
     *    "operation"?: string, //Filter by operation name
     *    "page"?: int, //Page number (0-based)
     *    "pageSize"?: int, //Page size
     *    "principalId"?: string, //Filter by principal ID
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
        return '/api/admin/audit-logs';
    }
    public function getBody(\Symfony\Component\Serializer\SerializerInterface $serializer, $streamFactory = null): array
    {
        return [[], null];
    }
    public function getExtraHeaders(): array
    {
        return ['Accept' => ['application/json']];
    }
    protected function getQueryOptionsResolver(): \Symfony\Component\OptionsResolver\OptionsResolver
    {
        $optionsResolver = parent::getQueryOptionsResolver();
        $optionsResolver->setDefined(['entityId', 'entityType', 'operation', 'page', 'pageSize', 'principalId']);
        $optionsResolver->setRequired([]);
        $optionsResolver->setDefaults(['page' => 0, 'pageSize' => 50]);
        $optionsResolver->addAllowedTypes('entityId', ['string']);
        $optionsResolver->addAllowedTypes('entityType', ['string']);
        $optionsResolver->addAllowedTypes('operation', ['string']);
        $optionsResolver->addAllowedTypes('page', ['int']);
        $optionsResolver->addAllowedTypes('pageSize', ['int']);
        $optionsResolver->addAllowedTypes('principalId', ['string']);
        return $optionsResolver;
    }
    /**
     * {@inheritdoc}
     *
     * @throws \FlowCatalyst\Generated\Exception\ListAuditLogsUnauthorizedException
     *
     * @return null|\FlowCatalyst\Generated\Model\AuditLogListResponse
     */
    protected function transformResponseBody(\Psr\Http\Message\ResponseInterface $response, \Symfony\Component\Serializer\SerializerInterface $serializer, ?string $contentType = null)
    {
        $status = $response->getStatusCode();
        $body = (string) $response->getBody();
        if (is_null($contentType) === false && (200 === $status && mb_strpos(strtolower($contentType), 'application/json') !== false)) {
            return $serializer->deserialize($body, 'FlowCatalyst\Generated\Model\AuditLogListResponse', 'json');
        }
        if (401 === $status) {
            throw new \FlowCatalyst\Generated\Exception\ListAuditLogsUnauthorizedException($response);
        }
    }
    public function getAuthenticationScopes(): array
    {
        return [];
    }
}