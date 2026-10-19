<?php

namespace FlowCatalyst\Generated\Endpoint;

class GetFunctionRunnerDesired extends \FlowCatalyst\Generated\Runtime\Client\BaseEndpoint implements \FlowCatalyst\Generated\Runtime\Client\Endpoint
{
    /**
     * @param array{
     *    "pool": string, //The runner pool to fetch desired state for
     *    "wait"?: int, //Seconds to hold the request open for a change, capped at control.MaxWait
     *    "runtimes"?: string, //Comma-separated runtimes the runner can run (for example wasm,js). Only versions of these runtimes are returned. Absent means wasm,js.
     * } $queryParameters
     * @param array{
     *    "If-None-Match"?: string,
     * } $headerParameters
     */
    public function __construct(array $queryParameters = [], array $headerParameters = [])
    {
        $this->queryParameters = $queryParameters;
        $this->headerParameters = $headerParameters;
    }
    use \FlowCatalyst\Generated\Runtime\Client\EndpointTrait;
    public function getMethod(): string
    {
        return 'GET';
    }
    public function getUri(): string
    {
        return '/control/functions/desired';
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
        $optionsResolver->setDefined(['pool', 'wait', 'runtimes']);
        $optionsResolver->setRequired(['pool']);
        $optionsResolver->setDefaults([]);
        $optionsResolver->addAllowedTypes('pool', ['string']);
        $optionsResolver->addAllowedTypes('wait', ['int']);
        $optionsResolver->addAllowedTypes('runtimes', ['string']);
        return $optionsResolver;
    }
    protected function getHeadersOptionsResolver(): \Symfony\Component\OptionsResolver\OptionsResolver
    {
        $optionsResolver = parent::getHeadersOptionsResolver();
        $optionsResolver->setDefined(['If-None-Match']);
        $optionsResolver->setRequired([]);
        $optionsResolver->setDefaults([]);
        $optionsResolver->addAllowedTypes('If-None-Match', ['string']);
        return $optionsResolver;
    }
    /**
     * {@inheritdoc}
     *
     *
     * @return null|\FlowCatalyst\Generated\Model\Desired|\FlowCatalyst\Generated\Model\ErrorModel
     */
    protected function transformResponseBody(\Psr\Http\Message\ResponseInterface $response, \Symfony\Component\Serializer\SerializerInterface $serializer, ?string $contentType = null)
    {
        $status = $response->getStatusCode();
        $body = (string) $response->getBody();
        if (is_null($contentType) === false && (200 === $status && mb_strpos(strtolower($contentType), 'application/json') !== false)) {
            return $serializer->deserialize($body, 'FlowCatalyst\Generated\Model\Desired', 'json');
        }
        if (mb_strpos(strtolower($contentType), 'application/json') !== false) {
            return $serializer->deserialize($body, 'FlowCatalyst\Generated\Model\ErrorModel', 'json');
        }
    }
    public function getAuthenticationScopes(): array
    {
        return [];
    }
}