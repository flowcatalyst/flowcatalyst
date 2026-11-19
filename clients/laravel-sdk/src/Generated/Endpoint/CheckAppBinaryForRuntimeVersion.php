<?php

namespace FlowCatalyst\Generated\Endpoint;

class CheckAppBinaryForRuntimeVersion extends \FlowCatalyst\Generated\Runtime\Client\BaseEndpoint implements \FlowCatalyst\Generated\Runtime\Client\Endpoint
{
    /**
     * @param array{
     *    "applicationCode": string,
     *    "appKey": string,
     *    "platform": string,
     *    "channel": string,
     *    "runtimeVersion": string,
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
        return '/api/app-updates/binary-check';
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
        $optionsResolver->setDefined(['applicationCode', 'appKey', 'platform', 'channel', 'runtimeVersion']);
        $optionsResolver->setRequired(['applicationCode', 'appKey', 'platform', 'channel', 'runtimeVersion']);
        $optionsResolver->setDefaults([]);
        $optionsResolver->addAllowedTypes('applicationCode', ['string']);
        $optionsResolver->addAllowedTypes('appKey', ['string']);
        $optionsResolver->addAllowedTypes('platform', ['string']);
        $optionsResolver->addAllowedTypes('channel', ['string']);
        $optionsResolver->addAllowedTypes('runtimeVersion', ['string']);
        return $optionsResolver;
    }
    protected function getQueryStyles(): array
    {
        return ['applicationCode' => ['style' => 'form', 'explode' => false], 'appKey' => ['style' => 'form', 'explode' => false], 'platform' => ['style' => 'form', 'explode' => false], 'channel' => ['style' => 'form', 'explode' => false], 'runtimeVersion' => ['style' => 'form', 'explode' => false]];
    }
    /**
     * {@inheritdoc}
     *
     *
     * @return null|\FlowCatalyst\Generated\Model\AppBinaryCheckResponse|\FlowCatalyst\Generated\Model\ErrorModel
     */
    protected function transformResponseBody(\Psr\Http\Message\ResponseInterface $response, \Symfony\Component\Serializer\SerializerInterface $serializer, ?string $contentType = null)
    {
        $status = $response->getStatusCode();
        $body = (string) $response->getBody();
        if (is_null($contentType) === false && (200 === $status && stripos(strtolower($contentType), 'application/json') !== false)) {
            return $serializer->deserialize($body, 'FlowCatalyst\Generated\Model\AppBinaryCheckResponse', 'json');
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