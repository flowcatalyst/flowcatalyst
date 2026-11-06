<?php

namespace FlowCatalyst\Generated\Endpoint;

class IngestionRuntimePoll extends \FlowCatalyst\Generated\Runtime\Client\BaseEndpoint implements \FlowCatalyst\Generated\Runtime\Client\Endpoint
{
    /**
     * @param null|\FlowCatalyst\Generated\Model\PollRequest $requestBody
     */
    public function __construct(?\FlowCatalyst\Generated\Model\PollRequest $requestBody = null)
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
        return '/api/ingestion-runtime/v1/poll';
    }
    public function getBody(\Symfony\Component\Serializer\SerializerInterface $serializer, $streamFactory = null): array
    {
        if ($this->body instanceof \FlowCatalyst\Generated\Model\PollRequest) {
            return [['Content-Type' => ['application/json']], \FlowCatalyst\Generated\Runtime\Client\JsonPayload::encode($serializer, $this->body)];
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
     * @throws \FlowCatalyst\Generated\Exception\IngestionRuntimePollConflictException
     * @throws \FlowCatalyst\Generated\Exception\IngestionRuntimePollUnprocessableEntityException
     * @throws \FlowCatalyst\Generated\Exception\IngestionRuntimePollUpgradeRequiredException
     * @throws \FlowCatalyst\Generated\Exception\IngestionRuntimePollInternalServerErrorException
     *
     * @return null|\FlowCatalyst\Generated\Model\PollResponse
     */
    protected function transformResponseBody(\Psr\Http\Message\ResponseInterface $response, \Symfony\Component\Serializer\SerializerInterface $serializer, ?string $contentType = null)
    {
        $status = $response->getStatusCode();
        $body = (string) $response->getBody();
        if (is_null($contentType) === false && (200 === $status && stripos(strtolower($contentType), 'application/json') !== false)) {
            return $serializer->deserialize($body, 'FlowCatalyst\Generated\Model\PollResponse', 'json');
        }
        if (is_null($contentType) === false && (409 === $status && stripos(strtolower($contentType), 'application/json') !== false)) {
            throw new \FlowCatalyst\Generated\Exception\IngestionRuntimePollConflictException($serializer->deserialize($body, 'FlowCatalyst\Generated\Model\ErrorModel', 'json'), $response);
        }
        if (is_null($contentType) === false && (422 === $status && stripos(strtolower($contentType), 'application/json') !== false)) {
            throw new \FlowCatalyst\Generated\Exception\IngestionRuntimePollUnprocessableEntityException($serializer->deserialize($body, 'FlowCatalyst\Generated\Model\ErrorModel', 'json'), $response);
        }
        if (is_null($contentType) === false && (426 === $status && stripos(strtolower($contentType), 'application/json') !== false)) {
            throw new \FlowCatalyst\Generated\Exception\IngestionRuntimePollUpgradeRequiredException($serializer->deserialize($body, 'FlowCatalyst\Generated\Model\ErrorModel', 'json'), $response);
        }
        if (is_null($contentType) === false && (500 === $status && stripos(strtolower($contentType), 'application/json') !== false)) {
            throw new \FlowCatalyst\Generated\Exception\IngestionRuntimePollInternalServerErrorException($serializer->deserialize($body, 'FlowCatalyst\Generated\Model\ErrorModel', 'json'), $response);
        }
    }
    public function getAuthenticationScopes(): array
    {
        return [];
    }
}