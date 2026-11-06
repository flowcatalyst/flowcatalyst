<?php

namespace FlowCatalyst\Generated\Endpoint;

class IngestionRuntimeReleaseLeases extends \FlowCatalyst\Generated\Runtime\Client\BaseEndpoint implements \FlowCatalyst\Generated\Runtime\Client\Endpoint
{
    /**
     * @param null|\FlowCatalyst\Generated\Model\ReleaseRequest $requestBody
     */
    public function __construct(?\FlowCatalyst\Generated\Model\ReleaseRequest $requestBody = null)
    {
        $this->body = $requestBody;
    }
    use \FlowCatalyst\Generated\Runtime\Client\EndpointTrait;
    public function getMethod(): string
    {
        return 'POST';
    }
    public function getUri(): string
    {
        return '/api/ingestion-runtime/v1/leases/release';
    }
    public function getBody(\Symfony\Component\Serializer\SerializerInterface $serializer, $streamFactory = null): array
    {
        if ($this->body instanceof \FlowCatalyst\Generated\Model\ReleaseRequest) {
            return [['Content-Type' => ['application/json']], $serializer->serialize($this->body, 'json')];
        }
        return [[], null];
    }
    public function getExtraHeaders(): array
    {
        return ['Accept' => ['application/json']];
    }
    /**
     * {@inheritdoc}
     *
     * @throws \FlowCatalyst\Generated\Exception\IngestionRuntimeReleaseLeasesConflictException
     * @throws \FlowCatalyst\Generated\Exception\IngestionRuntimeReleaseLeasesUnprocessableEntityException
     * @throws \FlowCatalyst\Generated\Exception\IngestionRuntimeReleaseLeasesInternalServerErrorException
     *
     * @return null|\FlowCatalyst\Generated\Model\ReleaseResponse
     */
    protected function transformResponseBody(\Psr\Http\Message\ResponseInterface $response, \Symfony\Component\Serializer\SerializerInterface $serializer, ?string $contentType = null)
    {
        $status = $response->getStatusCode();
        $body = (string) $response->getBody();
        if (is_null($contentType) === false && (200 === $status && mb_strpos(strtolower($contentType), 'application/json') !== false)) {
            return $serializer->deserialize($body, 'FlowCatalyst\Generated\Model\ReleaseResponse', 'json');
        }
        if (is_null($contentType) === false && (409 === $status && mb_strpos(strtolower($contentType), 'application/json') !== false)) {
            throw new \FlowCatalyst\Generated\Exception\IngestionRuntimeReleaseLeasesConflictException($serializer->deserialize($body, 'FlowCatalyst\Generated\Model\ErrorModel', 'json'), $response);
        }
        if (is_null($contentType) === false && (422 === $status && mb_strpos(strtolower($contentType), 'application/json') !== false)) {
            throw new \FlowCatalyst\Generated\Exception\IngestionRuntimeReleaseLeasesUnprocessableEntityException($serializer->deserialize($body, 'FlowCatalyst\Generated\Model\ErrorModel', 'json'), $response);
        }
        if (is_null($contentType) === false && (500 === $status && mb_strpos(strtolower($contentType), 'application/json') !== false)) {
            throw new \FlowCatalyst\Generated\Exception\IngestionRuntimeReleaseLeasesInternalServerErrorException($serializer->deserialize($body, 'FlowCatalyst\Generated\Model\ErrorModel', 'json'), $response);
        }
    }
    public function getAuthenticationScopes(): array
    {
        return [];
    }
}