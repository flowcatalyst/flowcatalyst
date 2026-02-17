<?php

namespace FlowCatalyst\Generated\Endpoint;

class PostApiBffEventTypesByIdSchema extends \FlowCatalyst\Generated\Runtime\Client\BaseEndpoint implements \FlowCatalyst\Generated\Runtime\Client\Endpoint
{
    protected $id;
    /**
     * @param string $id
     * @param null|\FlowCatalyst\Generated\Model\ApiBffEventTypesIdSchemasPostBody $requestBody
     */
    public function __construct(string $id, ?\FlowCatalyst\Generated\Model\ApiBffEventTypesIdSchemasPostBody $requestBody = null)
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
        return str_replace(['{id}'], [$this->id], '/api/bff/event-types/{id}/schemas');
    }
    public function getBody(\Symfony\Component\Serializer\SerializerInterface $serializer, $streamFactory = null): array
    {
        if ($this->body instanceof \FlowCatalyst\Generated\Model\ApiBffEventTypesIdSchemasPostBody) {
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
     * @throws \FlowCatalyst\Generated\Exception\PostApiBffEventTypesByIdSchemaBadRequestException
     * @throws \FlowCatalyst\Generated\Exception\PostApiBffEventTypesByIdSchemaNotFoundException
     * @throws \FlowCatalyst\Generated\Exception\PostApiBffEventTypesByIdSchemaConflictException
     *
     * @return null|\FlowCatalyst\Generated\Model\ApiBffEventTypesIdSchemasPostResponse201
     */
    protected function transformResponseBody(\Psr\Http\Message\ResponseInterface $response, \Symfony\Component\Serializer\SerializerInterface $serializer, ?string $contentType = null)
    {
        $status = $response->getStatusCode();
        $body = (string) $response->getBody();
        if (is_null($contentType) === false && (201 === $status && mb_strpos(strtolower($contentType), 'application/json') !== false)) {
            return $serializer->deserialize($body, 'FlowCatalyst\Generated\Model\ApiBffEventTypesIdSchemasPostResponse201', 'json');
        }
        if (is_null($contentType) === false && (400 === $status && mb_strpos(strtolower($contentType), 'application/json') !== false)) {
            throw new \FlowCatalyst\Generated\Exception\PostApiBffEventTypesByIdSchemaBadRequestException($serializer->deserialize($body, 'FlowCatalyst\Generated\Model\ApiBffEventTypesIdSchemasPostResponse400', 'json'), $response);
        }
        if (is_null($contentType) === false && (404 === $status && mb_strpos(strtolower($contentType), 'application/json') !== false)) {
            throw new \FlowCatalyst\Generated\Exception\PostApiBffEventTypesByIdSchemaNotFoundException($serializer->deserialize($body, 'FlowCatalyst\Generated\Model\ApiBffEventTypesIdSchemasPostResponse404', 'json'), $response);
        }
        if (is_null($contentType) === false && (409 === $status && mb_strpos(strtolower($contentType), 'application/json') !== false)) {
            throw new \FlowCatalyst\Generated\Exception\PostApiBffEventTypesByIdSchemaConflictException($serializer->deserialize($body, 'FlowCatalyst\Generated\Model\ApiBffEventTypesIdSchemasPostResponse409', 'json'), $response);
        }
    }
    public function getAuthenticationScopes(): array
    {
        return ['bearerAuth'];
    }
}