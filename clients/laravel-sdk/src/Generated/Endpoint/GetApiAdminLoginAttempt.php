<?php

namespace FlowCatalyst\Generated\Endpoint;

class GetApiAdminLoginAttempt extends \FlowCatalyst\Generated\Runtime\Client\BaseEndpoint implements \FlowCatalyst\Generated\Runtime\Client\Endpoint
{
    /**
     * @param array{
     *    "attemptType"?: string,
     *    "outcome"?: string,
     *    "identifier"?: string,
     *    "principalId"?: string,
     *    "dateFrom"?: string,
     *    "dateTo"?: string,
     *    "page"?: string,
     *    "pageSize"?: string,
     *    "sortField"?: string,
     *    "sortOrder"?: string,
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
        return '/api/admin/login-attempts';
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
        $optionsResolver->setDefined(['attemptType', 'outcome', 'identifier', 'principalId', 'dateFrom', 'dateTo', 'page', 'pageSize', 'sortField', 'sortOrder']);
        $optionsResolver->setRequired([]);
        $optionsResolver->setDefaults([]);
        $optionsResolver->addAllowedTypes('attemptType', ['string']);
        $optionsResolver->addAllowedTypes('outcome', ['string']);
        $optionsResolver->addAllowedTypes('identifier', ['string']);
        $optionsResolver->addAllowedTypes('principalId', ['string']);
        $optionsResolver->addAllowedTypes('dateFrom', ['string']);
        $optionsResolver->addAllowedTypes('dateTo', ['string']);
        $optionsResolver->addAllowedTypes('page', ['string']);
        $optionsResolver->addAllowedTypes('pageSize', ['string']);
        $optionsResolver->addAllowedTypes('sortField', ['string']);
        $optionsResolver->addAllowedTypes('sortOrder', ['string']);
        return $optionsResolver;
    }
    /**
     * {@inheritdoc}
     *
     *
     * @return null|\FlowCatalyst\Generated\Model\ApiAdminLoginAttemptsGetResponse200
     */
    protected function transformResponseBody(\Psr\Http\Message\ResponseInterface $response, \Symfony\Component\Serializer\SerializerInterface $serializer, ?string $contentType = null)
    {
        $status = $response->getStatusCode();
        $body = (string) $response->getBody();
        if (is_null($contentType) === false && (200 === $status && mb_strpos(strtolower($contentType), 'application/json') !== false)) {
            return $serializer->deserialize($body, 'FlowCatalyst\Generated\Model\ApiAdminLoginAttemptsGetResponse200', 'json');
        }
    }
    public function getAuthenticationScopes(): array
    {
        return ['bearerAuth'];
    }
}