<?php

namespace FlowCatalyst\Generated\Endpoint;

class SearchClients extends \FlowCatalyst\Generated\Runtime\Client\BaseEndpoint implements \FlowCatalyst\Generated\Runtime\Client\Endpoint
{
    /**
     * Search clients by name or identifier
     * @param array{
     *    "limit"?: int, //Max results to return
     *    "q"?: string, //Search query (name or identifier)
     *    "status"?: string, //Filter by status
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
        return '/api/admin/clients/search';
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
        $optionsResolver->setDefined(['limit', 'q', 'status']);
        $optionsResolver->setRequired([]);
        $optionsResolver->setDefaults(['limit' => 20]);
        $optionsResolver->addAllowedTypes('limit', ['int']);
        $optionsResolver->addAllowedTypes('q', ['string']);
        $optionsResolver->addAllowedTypes('status', ['string']);
        return $optionsResolver;
    }
    /**
     * {@inheritdoc}
     *
     * @throws \FlowCatalyst\Generated\Exception\SearchClientsUnauthorizedException
     * @throws \FlowCatalyst\Generated\Exception\SearchClientsForbiddenException
     *
     * @return null|\FlowCatalyst\Generated\Model\ClientListResponse1
     */
    protected function transformResponseBody(\Psr\Http\Message\ResponseInterface $response, \Symfony\Component\Serializer\SerializerInterface $serializer, ?string $contentType = null)
    {
        $status = $response->getStatusCode();
        $body = (string) $response->getBody();
        if (is_null($contentType) === false && (200 === $status && mb_strpos(strtolower($contentType), 'application/json') !== false)) {
            return $serializer->deserialize($body, 'FlowCatalyst\Generated\Model\ClientListResponse1', 'json');
        }
        if (401 === $status) {
            throw new \FlowCatalyst\Generated\Exception\SearchClientsUnauthorizedException($response);
        }
        if (403 === $status) {
            throw new \FlowCatalyst\Generated\Exception\SearchClientsForbiddenException($response);
        }
    }
    public function getAuthenticationScopes(): array
    {
        return [];
    }
}