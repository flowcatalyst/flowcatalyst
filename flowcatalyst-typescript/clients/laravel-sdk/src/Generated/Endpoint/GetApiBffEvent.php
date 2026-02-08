<?php

namespace FlowCatalyst\Generated\Endpoint;

class GetApiBffEvent extends \FlowCatalyst\Generated\Runtime\Client\BaseEndpoint implements \FlowCatalyst\Generated\Runtime\Client\Endpoint
{
    /**
     * Search events with optional filters and pagination. Multi-value parameters (clientIds, applications, etc.) support comma-separated values for OR filtering. Use 'null' in clientIds to include platform events (no client).
     * @param array{
     *    "aggregates"?: string,
     *    "applications"?: string,
     *    "clientIds"?: string,
     *    "correlationId"?: string,
     *    "messageGroup"?: string,
     *    "page"?: int,
     *    "size"?: int,
     *    "source"?: string,
     *    "subdomains"?: string,
     *    "subject"?: string,
     *    "timeAfter"?: string,
     *    "timeBefore"?: string,
     *    "types"?: string,
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
        return '/api/bff/events';
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
        $optionsResolver->setDefined(['aggregates', 'applications', 'clientIds', 'correlationId', 'messageGroup', 'page', 'size', 'source', 'subdomains', 'subject', 'timeAfter', 'timeBefore', 'types']);
        $optionsResolver->setRequired([]);
        $optionsResolver->setDefaults(['page' => '0', 'size' => '20']);
        $optionsResolver->addAllowedTypes('aggregates', ['string']);
        $optionsResolver->addAllowedTypes('applications', ['string']);
        $optionsResolver->addAllowedTypes('clientIds', ['string']);
        $optionsResolver->addAllowedTypes('correlationId', ['string']);
        $optionsResolver->addAllowedTypes('messageGroup', ['string']);
        $optionsResolver->addAllowedTypes('page', ['int']);
        $optionsResolver->addAllowedTypes('size', ['int']);
        $optionsResolver->addAllowedTypes('source', ['string']);
        $optionsResolver->addAllowedTypes('subdomains', ['string']);
        $optionsResolver->addAllowedTypes('subject', ['string']);
        $optionsResolver->addAllowedTypes('timeAfter', ['string']);
        $optionsResolver->addAllowedTypes('timeBefore', ['string']);
        $optionsResolver->addAllowedTypes('types', ['string']);
        return $optionsResolver;
    }
    /**
     * {@inheritdoc}
     *
     *
     * @return null|\FlowCatalyst\Generated\Model\PagedEventResponse
     */
    protected function transformResponseBody(\Psr\Http\Message\ResponseInterface $response, \Symfony\Component\Serializer\SerializerInterface $serializer, ?string $contentType = null)
    {
        $status = $response->getStatusCode();
        $body = (string) $response->getBody();
        if (is_null($contentType) === false && (200 === $status && mb_strpos(strtolower($contentType), 'application/json') !== false)) {
            return $serializer->deserialize($body, 'FlowCatalyst\Generated\Model\PagedEventResponse', 'json');
        }
    }
    public function getAuthenticationScopes(): array
    {
        return [];
    }
}