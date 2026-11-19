<?php

namespace FlowCatalyst\Generated\Endpoint;

class ListAppBinaries extends \FlowCatalyst\Generated\Runtime\Client\BaseEndpoint implements \FlowCatalyst\Generated\Runtime\Client\Endpoint
{
    /**
     * @param array{
     *    "applicationCode": string,
     *    "appKey"?: string,
     *    "platform"?: string,
     *    "channel"?: string,
     *    "variant"?: string,
     *    "runtimeVersion"?: string,
     *    "page"?: int,
     *    "size"?: int,
     *    "limit"?: int,
     *    "pageSize"?: int,
     *    "page_size"?: int,
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
        return '/api/app-updates/binaries';
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
        $optionsResolver->setDefined(['applicationCode', 'appKey', 'platform', 'channel', 'variant', 'runtimeVersion', 'page', 'size', 'limit', 'pageSize', 'page_size']);
        $optionsResolver->setRequired(['applicationCode']);
        $optionsResolver->setDefaults([]);
        $optionsResolver->addAllowedTypes('applicationCode', ['string']);
        $optionsResolver->addAllowedTypes('appKey', ['string']);
        $optionsResolver->addAllowedTypes('platform', ['string']);
        $optionsResolver->addAllowedTypes('channel', ['string']);
        $optionsResolver->addAllowedTypes('variant', ['string']);
        $optionsResolver->addAllowedTypes('runtimeVersion', ['string']);
        $optionsResolver->addAllowedTypes('page', ['int']);
        $optionsResolver->addAllowedTypes('size', ['int']);
        $optionsResolver->addAllowedTypes('limit', ['int']);
        $optionsResolver->addAllowedTypes('pageSize', ['int']);
        $optionsResolver->addAllowedTypes('page_size', ['int']);
        return $optionsResolver;
    }
    protected function getQueryStyles(): array
    {
        return ['applicationCode' => ['style' => 'form', 'explode' => false], 'appKey' => ['style' => 'form', 'explode' => false], 'platform' => ['style' => 'form', 'explode' => false], 'channel' => ['style' => 'form', 'explode' => false], 'variant' => ['style' => 'form', 'explode' => false], 'runtimeVersion' => ['style' => 'form', 'explode' => false], 'page' => ['style' => 'form', 'explode' => false], 'size' => ['style' => 'form', 'explode' => false], 'limit' => ['style' => 'form', 'explode' => false], 'pageSize' => ['style' => 'form', 'explode' => false], 'page_size' => ['style' => 'form', 'explode' => false]];
    }
    /**
     * {@inheritdoc}
     *
     *
     * @return null|\FlowCatalyst\Generated\Model\OffsetPageAppBinaryResponse|\FlowCatalyst\Generated\Model\ErrorModel
     */
    protected function transformResponseBody(\Psr\Http\Message\ResponseInterface $response, \Symfony\Component\Serializer\SerializerInterface $serializer, ?string $contentType = null)
    {
        $status = $response->getStatusCode();
        $body = (string) $response->getBody();
        if (is_null($contentType) === false && (200 === $status && stripos(strtolower($contentType), 'application/json') !== false)) {
            return $serializer->deserialize($body, 'FlowCatalyst\Generated\Model\OffsetPageAppBinaryResponse', 'json');
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