<?php

namespace FlowCatalyst\Generated\Endpoint;

class IngestionRuntimeGetSecret extends \FlowCatalyst\Generated\Runtime\Client\BaseEndpoint implements \FlowCatalyst\Generated\Runtime\Client\Endpoint
{
    protected $id;
    protected $name;
    /**
     * @param string $id
     * @param string $name A secret the ingestion's live definition names
     * @param array{
     *    "runtimeId": string, //The asking runtime, which must carry the ingestion's kind and for a polled source hold its lease
     * } $queryParameters
     */
    public function __construct(string $id, string $name, array $queryParameters = [])
    {
        $this->id = $id;
        $this->name = $name;
        $this->queryParameters = $queryParameters;
    }
    use \FlowCatalyst\Generated\Runtime\Client\EndpointTrait;
    public function getMethod(): string
    {
        return 'GET';
    }
    public function getUri(): string
    {
        return str_replace(['{id}', '{name}'], [rawurlencode($this->id), rawurlencode($this->name)], '/api/ingestion-runtime/v1/ingestions/{id}/secrets/{name}');
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
        $optionsResolver->setDefined(['runtimeId']);
        $optionsResolver->setRequired(['runtimeId']);
        $optionsResolver->setDefaults([]);
        $optionsResolver->addAllowedTypes('runtimeId', ['string']);
        return $optionsResolver;
    }
    protected function getQueryStyles(): array
    {
        return ['runtimeId' => ['style' => 'form', 'explode' => false]];
    }
    /**
     * {@inheritdoc}
     *
     *
     * @return null|\FlowCatalyst\Generated\Model\SecretValue|\FlowCatalyst\Generated\Model\ErrorModel
     */
    protected function transformResponseBody(\Psr\Http\Message\ResponseInterface $response, \Symfony\Component\Serializer\SerializerInterface $serializer, ?string $contentType = null)
    {
        $status = $response->getStatusCode();
        $body = (string) $response->getBody();
        if (is_null($contentType) === false && (200 === $status && stripos(strtolower($contentType), 'application/json') !== false)) {
            return $serializer->deserialize($body, 'FlowCatalyst\Generated\Model\SecretValue', 'json');
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