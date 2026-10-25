<?php

namespace FlowCatalyst\Generated\Endpoint;

class IngestionRuntimePutState extends \FlowCatalyst\Generated\Runtime\Client\BaseEndpoint implements \FlowCatalyst\Generated\Runtime\Client\Endpoint
{
    protected $ingestionId;
    protected $key;
    /**
     * @param string $ingestionId
     * @param string $key
     * @param null|\FlowCatalyst\Generated\Model\StateValue $requestBody
     * @param array{
     *    "runtimeId": string, //The writing runtime, which must hold the ingestion's lease
     * } $queryParameters
     */
    public function __construct(string $ingestionId, string $key, ?\FlowCatalyst\Generated\Model\StateValue $requestBody = null, array $queryParameters = [])
    {
        $this->ingestionId = $ingestionId;
        $this->key = $key;
        $this->body = $requestBody;
        $this->queryParameters = $queryParameters;
    }
    use \FlowCatalyst\Generated\Runtime\Client\EndpointTrait;
    public function getMethod(): string
    {
        return 'PUT';
    }
    public function getUri(): string
    {
        return str_replace(['{ingestionId}', '{key}'], [$this->ingestionId, $this->key], '/api/ingestion-runtime/v1/state/{ingestionId}/{key}');
    }
    public function getBody(\Symfony\Component\Serializer\SerializerInterface $serializer, $streamFactory = null): array
    {
        if ($this->body instanceof \FlowCatalyst\Generated\Model\StateValue) {
            return [['Content-Type' => ['application/json']], $serializer->serialize($this->body, 'json')];
        }
        return [[], null];
    }
    public function getExtraHeaders(): array
    {
        return ['Accept' => ['application/json']];
    }
    protected function getQueryOptionsResolver(): \Symfony\Component\OptionsResolver\OptionsResolver
    {
        $optionsResolver = parent::getQueryOptionsResolver();
        $optionsResolver->setDefined(['runtimeId']);
        $optionsResolver->setRequired(['runtimeId']);
        $optionsResolver->setDefaults([]);
        $optionsResolver->addAllowedTypes('runtimeId', ['string']);
        return $optionsResolver;
    }
    /**
     * {@inheritdoc}
     *
     *
     * @return null|\FlowCatalyst\Generated\Model\ErrorModel
     */
    protected function transformResponseBody(\Psr\Http\Message\ResponseInterface $response, \Symfony\Component\Serializer\SerializerInterface $serializer, ?string $contentType = null)
    {
        $status = $response->getStatusCode();
        $body = (string) $response->getBody();
        if (204 === $status) {
            return null;
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