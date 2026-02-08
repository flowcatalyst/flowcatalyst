<?php

namespace FlowCatalyst\Generated\Endpoint;

class PostApiEventTypesByIdSchema extends \FlowCatalyst\Generated\Runtime\Client\BaseEndpoint implements \FlowCatalyst\Generated\Runtime\Client\Endpoint
{
    protected $id;
    /**
     * @param string $id
     * @param null|\FlowCatalyst\Generated\Model\AddSchemaRequest $requestBody
     */
    public function __construct(string $id, ?\FlowCatalyst\Generated\Model\AddSchemaRequest $requestBody = null)
    {
        $this->id = $id;
        $this->body = $requestBody;
    }
    use \FlowCatalyst\Generated\Runtime\Client\EndpointTrait;
    public function getMethod(): string
    {
        return 'POST';
    }
    public function getUri(): string
    {
        return str_replace(['{id}'], [$this->id], '/api/event-types/{id}/schemas');
    }
    public function getBody(\Symfony\Component\Serializer\SerializerInterface $serializer, $streamFactory = null): array
    {
        if ($this->body instanceof \FlowCatalyst\Generated\Model\AddSchemaRequest) {
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
     * @throws \FlowCatalyst\Generated\Exception\PostApiEventTypesByIdSchemaBadRequestException
     * @throws \FlowCatalyst\Generated\Exception\PostApiEventTypesByIdSchemaNotFoundException
     * @throws \FlowCatalyst\Generated\Exception\PostApiEventTypesByIdSchemaConflictException
     *
     * @return null|\FlowCatalyst\Generated\Model\EventTypeResponse
     */
    protected function transformResponseBody(\Psr\Http\Message\ResponseInterface $response, \Symfony\Component\Serializer\SerializerInterface $serializer, ?string $contentType = null)
    {
        $status = $response->getStatusCode();
        $body = (string) $response->getBody();
        if (is_null($contentType) === false && (201 === $status && mb_strpos(strtolower($contentType), 'application/json') !== false)) {
            return $serializer->deserialize($body, 'FlowCatalyst\Generated\Model\EventTypeResponse', 'json');
        }
        if (400 === $status) {
            throw new \FlowCatalyst\Generated\Exception\PostApiEventTypesByIdSchemaBadRequestException($response);
        }
        if (404 === $status) {
            throw new \FlowCatalyst\Generated\Exception\PostApiEventTypesByIdSchemaNotFoundException($response);
        }
        if (409 === $status) {
            throw new \FlowCatalyst\Generated\Exception\PostApiEventTypesByIdSchemaConflictException($response);
        }
    }
    public function getAuthenticationScopes(): array
    {
        return [];
    }
}