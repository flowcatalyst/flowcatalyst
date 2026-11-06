<?php

namespace FlowCatalyst\Generated\Endpoint;

class ListLoginAttempts extends \FlowCatalyst\Generated\Runtime\Client\BaseEndpoint implements \FlowCatalyst\Generated\Runtime\Client\Endpoint
{
    /**
     * @param array{
     *    "attemptType"?: string,
     *    "outcome"?: string,
     *    "identifier"?: string,
     *    "principalId"?: string,
     *    "dateFrom"?: string,
     *    "dateTo"?: string,
     *    "after"?: string,
     *    "pageSize"?: int,
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
        return '/api/login-attempts';
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
        $optionsResolver->setDefined(['attemptType', 'outcome', 'identifier', 'principalId', 'dateFrom', 'dateTo', 'after', 'pageSize']);
        $optionsResolver->setRequired([]);
        $optionsResolver->setDefaults([]);
        $optionsResolver->addAllowedTypes('attemptType', ['string']);
        $optionsResolver->addAllowedTypes('outcome', ['string']);
        $optionsResolver->addAllowedTypes('identifier', ['string']);
        $optionsResolver->addAllowedTypes('principalId', ['string']);
        $optionsResolver->addAllowedTypes('dateFrom', ['string']);
        $optionsResolver->addAllowedTypes('dateTo', ['string']);
        $optionsResolver->addAllowedTypes('after', ['string']);
        $optionsResolver->addAllowedTypes('pageSize', ['int']);
        return $optionsResolver;
    }
    protected function getQueryStyles(): array
    {
        return ['attemptType' => ['style' => 'form', 'explode' => false], 'outcome' => ['style' => 'form', 'explode' => false], 'identifier' => ['style' => 'form', 'explode' => false], 'principalId' => ['style' => 'form', 'explode' => false], 'dateFrom' => ['style' => 'form', 'explode' => false], 'dateTo' => ['style' => 'form', 'explode' => false], 'after' => ['style' => 'form', 'explode' => false], 'pageSize' => ['style' => 'form', 'explode' => false]];
    }
    /**
     * {@inheritdoc}
     *
     *
     * @return null|\FlowCatalyst\Generated\Model\LoginAttemptListResponse|\FlowCatalyst\Generated\Model\ErrorModel
     */
    protected function transformResponseBody(\Psr\Http\Message\ResponseInterface $response, \Symfony\Component\Serializer\SerializerInterface $serializer, ?string $contentType = null)
    {
        $status = $response->getStatusCode();
        $body = (string) $response->getBody();
        if (is_null($contentType) === false && (200 === $status && stripos(strtolower($contentType), 'application/json') !== false)) {
            return $serializer->deserialize($body, 'FlowCatalyst\Generated\Model\LoginAttemptListResponse', 'json');
        }
        if (stripos(strtolower($contentType), 'application/json') !== false) {
            return $serializer->deserialize($body, 'FlowCatalyst\Generated\Model\ErrorModel', 'json');
        }
    }
    public function getAuthenticationScopes(): array
    {
        return [];
    }
}