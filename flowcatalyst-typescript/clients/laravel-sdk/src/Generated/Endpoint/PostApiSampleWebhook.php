<?php

namespace FlowCatalyst\Generated\Endpoint;

class PostApiSampleWebhook extends \FlowCatalyst\Generated\Runtime\Client\BaseEndpoint implements \FlowCatalyst\Generated\Runtime\Client\Endpoint
{
    /**
     * @param null|string $requestBody
     * @param array{
     *    "X-FlowCatalyst-Causation-ID"?: string,
     *    "X-FlowCatalyst-Code"?: string,
     *    "X-FlowCatalyst-Correlation-ID"?: string,
     *    "X-FlowCatalyst-ID"?: string,
     *    "X-FlowCatalyst-Kind"?: string,
     *    "X-FlowCatalyst-Signature"?: string,
     *    "X-FlowCatalyst-Subject"?: string,
     *    "X-FlowCatalyst-Timestamp"?: string,
     * } $headerParameters
     */
    public function __construct(?string $requestBody = null, array $headerParameters = [])
    {
        $this->body = $requestBody;
        $this->headerParameters = $headerParameters;
    }
    use \FlowCatalyst\Generated\Runtime\Client\EndpointTrait;
    public function getMethod(): string
    {
        return 'POST';
    }
    public function getUri(): string
    {
        return '/api/sample/webhook';
    }
    public function getBody(\Symfony\Component\Serializer\SerializerInterface $serializer, $streamFactory = null): array
    {
        if (is_string($this->body)) {
            return [['Content-Type' => ['application/json']], $serializer->serialize($this->body, 'json')];
        }
        return [[], null];
    }
    public function getExtraHeaders(): array
    {
        return ['Accept' => ['application/json']];
    }
    protected function getHeadersOptionsResolver(): \Symfony\Component\OptionsResolver\OptionsResolver
    {
        $optionsResolver = parent::getHeadersOptionsResolver();
        $optionsResolver->setDefined(['X-FlowCatalyst-Causation-ID', 'X-FlowCatalyst-Code', 'X-FlowCatalyst-Correlation-ID', 'X-FlowCatalyst-ID', 'X-FlowCatalyst-Kind', 'X-FlowCatalyst-Signature', 'X-FlowCatalyst-Subject', 'X-FlowCatalyst-Timestamp']);
        $optionsResolver->setRequired([]);
        $optionsResolver->setDefaults([]);
        $optionsResolver->addAllowedTypes('X-FlowCatalyst-Causation-ID', ['string']);
        $optionsResolver->addAllowedTypes('X-FlowCatalyst-Code', ['string']);
        $optionsResolver->addAllowedTypes('X-FlowCatalyst-Correlation-ID', ['string']);
        $optionsResolver->addAllowedTypes('X-FlowCatalyst-ID', ['string']);
        $optionsResolver->addAllowedTypes('X-FlowCatalyst-Kind', ['string']);
        $optionsResolver->addAllowedTypes('X-FlowCatalyst-Signature', ['string']);
        $optionsResolver->addAllowedTypes('X-FlowCatalyst-Subject', ['string']);
        $optionsResolver->addAllowedTypes('X-FlowCatalyst-Timestamp', ['string']);
        return $optionsResolver;
    }
    /**
     * {@inheritdoc}
     *
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
    }
    public function getAuthenticationScopes(): array
    {
        return [];
    }
}