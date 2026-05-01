<?php

namespace FlowCatalyst\Generated\Endpoint;

class GetApiAdminEvents extends \FlowCatalyst\Generated\Runtime\Client\BaseEndpoint implements \FlowCatalyst\Generated\Runtime\Client\Endpoint
{
    /**
     * @param array{
     *    "page"?: int, //Page number (1-based). Falls back to 1.
     *    "size"?: int, //Page size. Capped at 500.
     *    "sortField"?: string, //Sort field. Allow-listed; unknown values fall back to `time`.
     *    "sortOrder"?: string, //Sort order: `asc` or `desc`. Defaults to `desc`.
     *    "clientIds"?: string, //Filter by client IDs (comma-separated)
     *    "types"?: string, //Filter by event types (comma-separated)
     *    "applications"?: string, //Filter by application codes (comma-separated)
     *    "subdomains"?: string, //Filter by subdomains (comma-separated)
     *    "aggregates"?: string, //Filter by aggregates (comma-separated)
     *    "correlationId"?: string, //Filter by correlation ID
     *    "source"?: string, //Free-text search across type, source, subject
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
        return '/bff/events';
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
        $optionsResolver->setDefined(['page', 'size', 'sortField', 'sortOrder', 'clientIds', 'types', 'applications', 'subdomains', 'aggregates', 'correlationId', 'source']);
        $optionsResolver->setRequired([]);
        $optionsResolver->setDefaults([]);
        $optionsResolver->addAllowedTypes('page', ['int']);
        $optionsResolver->addAllowedTypes('size', ['int']);
        $optionsResolver->addAllowedTypes('sortField', ['string']);
        $optionsResolver->addAllowedTypes('sortOrder', ['string']);
        $optionsResolver->addAllowedTypes('clientIds', ['string']);
        $optionsResolver->addAllowedTypes('types', ['string']);
        $optionsResolver->addAllowedTypes('applications', ['string']);
        $optionsResolver->addAllowedTypes('subdomains', ['string']);
        $optionsResolver->addAllowedTypes('aggregates', ['string']);
        $optionsResolver->addAllowedTypes('correlationId', ['string']);
        $optionsResolver->addAllowedTypes('source', ['string']);
        return $optionsResolver;
    }
    /**
     * {@inheritdoc}
     *
     *
     * @return null|\FlowCatalyst\Generated\Model\PagedEventsResponse
     */
    protected function transformResponseBody(\Psr\Http\Message\ResponseInterface $response, \Symfony\Component\Serializer\SerializerInterface $serializer, ?string $contentType = null)
    {
        $status = $response->getStatusCode();
        $body = (string) $response->getBody();
        if (is_null($contentType) === false && (200 === $status && mb_strpos(strtolower($contentType), 'application/json') !== false)) {
            return $serializer->deserialize($body, 'FlowCatalyst\Generated\Model\PagedEventsResponse', 'json');
        }
    }
    public function getAuthenticationScopes(): array
    {
        return ['bearer_auth'];
    }
}