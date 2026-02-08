<?php

namespace FlowCatalyst\Generated\Endpoint;

class GetApiBffDispatchJobsFilterOption extends \FlowCatalyst\Generated\Runtime\Client\BaseEndpoint implements \FlowCatalyst\Generated\Runtime\Client\Endpoint
{
    /**
     * Get available filter values for cascading filters. Each level is narrowed by selections at higher levels (client → application → subdomain → aggregate → code).
     * @param array{
     *    "aggregates"?: string,
     *    "applications"?: string,
     *    "clientIds"?: string,
     *    "subdomains"?: string,
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
        return '/api/bff/dispatch-jobs/filter-options';
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
        $optionsResolver->setDefined(['aggregates', 'applications', 'clientIds', 'subdomains']);
        $optionsResolver->setRequired([]);
        $optionsResolver->setDefaults([]);
        $optionsResolver->addAllowedTypes('aggregates', ['string']);
        $optionsResolver->addAllowedTypes('applications', ['string']);
        $optionsResolver->addAllowedTypes('clientIds', ['string']);
        $optionsResolver->addAllowedTypes('subdomains', ['string']);
        return $optionsResolver;
    }
    /**
     * {@inheritdoc}
     *
     *
     * @return null|\FlowCatalyst\Generated\Model\FilterOptionsResponse1
     */
    protected function transformResponseBody(\Psr\Http\Message\ResponseInterface $response, \Symfony\Component\Serializer\SerializerInterface $serializer, ?string $contentType = null)
    {
        $status = $response->getStatusCode();
        $body = (string) $response->getBody();
        if (is_null($contentType) === false && (200 === $status && mb_strpos(strtolower($contentType), 'application/json') !== false)) {
            return $serializer->deserialize($body, 'FlowCatalyst\Generated\Model\FilterOptionsResponse1', 'json');
        }
    }
    public function getAuthenticationScopes(): array
    {
        return [];
    }
}